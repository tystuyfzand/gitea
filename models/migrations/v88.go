// Copyright 2019 Gitea. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"github.com/go-xorm/xorm"
)

func addIsCupFieldToRepository(x *xorm.Engine) error {
	type Repository struct {
		// ID(10-20)-md5(32) - must fit into 64 symbols
		IsCup bool `xorm:"DEFAULT(false)"`
	}

	if err := x.Sync2(new(Repository)); err != nil {
		return err
	}

	_, err := x.Exec("UPDATE repository SET is_cup = ?", false)
	return err
}
