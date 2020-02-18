// Copyright 2016 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package code

import (
	"context"
	"os"
	"time"

	"code.gitea.io/gitea/models"
	"code.gitea.io/gitea/modules/graceful"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/queue"
	"code.gitea.io/gitea/modules/setting"
)

// SearchResult result of performing a search in a repo
type SearchResult struct {
	RepoID     int64
	StartIndex int
	EndIndex   int
	Filename   string
	Content    string
}

// Indexer defines an interface to indexer issues contents
type Indexer interface {
	Index(repoID int64) error
	Delete(repoID int64) error
	Search(repoIDs []int64, keyword string, page, pageSize int) (int64, []*SearchResult, error)
	Close()
}

type IndexerData struct {
	RepoID   int64
	IsDelete bool
}

var (
	indexerQueue queue.Queue
)

// Init initialize the repo indexer
func Init() {
	if !setting.Indexer.RepoIndexerEnabled {
		indexer.Close()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	graceful.GetManager().RunAtTerminate(ctx, func() {
		log.Debug("Closing repository indexer")
		indexer.Close()
		log.Info("PID: %d Repository Indexer closed", os.Getpid())
	})

	waitChannel := make(chan time.Duration)

	// Create the Queue
	switch setting.Indexer.RepoType {
	case "bleve":
		handler := func(data ...queue.Data) {
			idx, err := indexer.get()
			if idx == nil || err != nil {
				log.Error("Codes indexer handler: unable to get indexer!")
				return
			}

			for _, datum := range data {
				indexerData, ok := datum.(*IndexerData)
				if !ok {
					log.Error("Unable to process provided datum: %v - not possible to cast to IndexerData", datum)
					continue
				}
				log.Trace("IndexerData Process: %d %v %t", indexerData.RepoID, indexerData.IsDelete)

				if indexerData.IsDelete {
					if err := indexer.Delete(indexerData.RepoID); err != nil {
						log.Error("indexer.Delete: %v", err)
					}
				} else {
					if err := indexer.Index(indexerData.RepoID); err != nil {
						log.Error("indexer.Index: %v", err)
					}
				}
			}
		}

		indexerQueue = queue.CreateQueue("code_indexer", handler, &IndexerData{})
		if indexerQueue == nil {
			log.Fatal("Unable to create codes indexer queue")
		}
	default:
		log.Fatal("Unknown codes indexer type; %s", setting.Indexer.RepoType)
	}

	go func() {
		start := time.Now()
		log.Info("PID: %d Initializing Repository Indexer at: %s", os.Getpid(), setting.Indexer.RepoPath)
		bleveIndexer, created, err := NewBleveIndexer(setting.Indexer.RepoPath)
		if err != nil {
			if bleveIndexer != nil {
				bleveIndexer.Close()
			}
			cancel()
			indexer.Close()
			close(waitChannel)
			log.Fatal("PID: %d Unable to initialize the Repository Indexer at path: %s Error: %v", os.Getpid(), setting.Indexer.RepoPath, err)
		}
		indexer.set(bleveIndexer)

		if created {
			go populateRepoIndexer()
		}
		select {
		case waitChannel <- time.Since(start):
		case <-graceful.GetManager().IsShutdown():
		}

		close(waitChannel)
	}()

	if setting.Indexer.StartupTimeout > 0 {
		go func() {
			timeout := setting.Indexer.StartupTimeout
			if graceful.GetManager().IsChild() && setting.GracefulHammerTime > 0 {
				timeout += setting.GracefulHammerTime
			}
			select {
			case <-graceful.GetManager().IsShutdown():
				log.Warn("Shutdown before Repository Indexer completed initialization")
				cancel()
				indexer.Close()
			case duration, ok := <-waitChannel:
				if !ok {
					log.Warn("Repository Indexer Initialization failed")
					cancel()
					indexer.Close()
					return
				}
				log.Info("Repository Indexer Initialization took %v", duration)
			case <-time.After(timeout):
				cancel()
				indexer.Close()
				log.Fatal("Repository Indexer Initialization Timed-Out after: %v", timeout)
			}
		}()
	}
}

// DeleteRepoFromIndexer remove all of a repository's entries from the indexer
func DeleteRepoFromIndexer(repo *models.Repository) {
	indexerQueue.Push(&IndexerData{RepoID: repo.ID, IsDelete: true})
}

// UpdateRepoIndexer update a repository's entries in the indexer
func UpdateRepoIndexer(repo *models.Repository) {
	indexerQueue.Push(&IndexerData{RepoID: repo.ID, IsDelete: false})
}

// populateRepoIndexer populate the repo indexer with pre-existing data. This
// should only be run when the indexer is created for the first time.
func populateRepoIndexer() {
	log.Info("Populating the repo indexer with existing repositories")

	isShutdown := graceful.GetManager().IsShutdown()

	exist, err := models.IsTableNotEmpty("repository")
	if err != nil {
		log.Fatal("System error: %v", err)
	} else if !exist {
		return
	}

	// if there is any existing repo indexer metadata in the DB, delete it
	// since we are starting afresh. Also, xorm requires deletes to have a
	// condition, and we want to delete everything, thus 1=1.
	if err := models.DeleteAllRecords("repo_indexer_status"); err != nil {
		log.Fatal("System error: %v", err)
	}

	var maxRepoID int64
	if maxRepoID, err = models.GetMaxID("repository"); err != nil {
		log.Fatal("System error: %v", err)
	}

	// start with the maximum existing repo ID and work backwards, so that we
	// don't include repos that are created after gitea starts; such repos will
	// already be added to the indexer, and we don't need to add them again.
	for maxRepoID > 0 {
		select {
		case <-isShutdown:
			log.Info("Repository Indexer population shutdown before completion")
			return
		default:
		}
		ids, err := models.GetUnindexedRepos(models.RepoIndexerTypeCode, maxRepoID, 0, 50)
		if err != nil {
			log.Error("populateRepoIndexer: %v", err)
			return
		} else if len(ids) == 0 {
			break
		}
		for _, id := range ids {
			select {
			case <-isShutdown:
				log.Info("Repository Indexer population shutdown before completion")
				return
			default:
			}
			indexerQueue.Push(&IndexerData{RepoID: id, IsDelete: false})
			maxRepoID = id - 1
		}
	}
	log.Info("Done (re)populating the repo indexer with existing repositories")
}
