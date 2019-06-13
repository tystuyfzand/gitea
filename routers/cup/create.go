// Copyright 2019 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cup

import (
	"code.gitea.io/gitea/modules/context"
	"code.gitea.io/gitea/modules/base"
)

const (
	tplCupNew    base.TplName = "cup/create"
)

// CreateCup represents the cup creation page
func CreateCup(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("cup.new")
	ctx.Data["RequireSimpleMDE"] = true
	ctx.HTML(200, tplCupNew)
}

// EditCup represents the cup editing page
func EditCup(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("cup.edit")
	ctx.Data["PageIsCupEdit"] = true
	ctx.Data["RequireSimpleMDE"] = true
	ctx.HTML(200, tplCupNew)
}

// CreateCupPost represents the process of creating cup
func CreateCupPost(ctx *context.Context) {

}