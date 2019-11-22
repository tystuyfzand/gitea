// Copyright 2018 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package ui

import (
	"code.gitea.io/gitea/models"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/notification/base"
)

type (
	notificationService struct {
		base.NullNotifier
		queue chan models.NotificationOpts
	}
)

var (
	_ base.Notifier = &notificationService{}
)

// NewNotifier create a new notificationService notifier
func NewNotifier() base.Notifier {
	return &notificationService{
		queue: make(chan models.NotificationOpts, 100),
	}
}

func (ns *notificationService) Run() {
	for opts := range ns.queue {
		if err := models.CreateOrUpdateIssueNotifications(opts); err != nil {
			log.Error("Was unable to create issue notification: %v", err)
		}
	}
}

func (ns *notificationService) NotifyCreateIssueComment(doer *models.User, repo *models.Repository,
	issue *models.Issue, comment *models.Comment) {
	var opts = models.NotificationOpts{
		IssueID: issue.ID,
		DoerID:  doer.ID,
	}
	if comment != nil {
		opts.CommentID = comment.ID
	}
	ns.queue <- opts
}

func (ns *notificationService) NotifyNewIssue(issue *models.Issue) {
	ns.queue <- models.NotificationOpts{
		IssueID: issue.ID,
		DoerID:  issue.Poster.ID,
	}
}

func (ns *notificationService) NotifyIssueChangeStatus(doer *models.User, issue *models.Issue, isClosed bool) {
	ns.queue <- models.NotificationOpts{
		IssueID: issue.ID,
		DoerID:  doer.ID,
	}
}

func (ns *notificationService) NotifyMergePullRequest(pr *models.PullRequest, doer *models.User, gitRepo *git.Repository) {
	ns.queue <- models.NotificationOpts{
		IssueID: pr.Issue.ID,
		DoerID:  doer.ID,
	}
}

func (ns *notificationService) NotifyNewPullRequest(pr *models.PullRequest) {
	ns.queue <- models.NotificationOpts{
		IssueID: pr.Issue.ID,
		DoerID:  pr.Issue.PosterID,
	}
}

func (ns *notificationService) NotifyPullRequestReview(pr *models.PullRequest, r *models.Review, c *models.Comment) {
	var opts = models.NotificationOpts{
		IssueID: pr.Issue.ID,
		DoerID:  r.Reviewer.ID,
	}
	if c != nil {
		opts.CommentID = c.ID
	}
	ns.queue <- opts
}
