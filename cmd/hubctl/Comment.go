// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

func commentGroup() group {
	return group{
		name:    "comment",
		summary: "the conversation on an entry",
		commands: []command{
			{
				name:    "ls",
				usage:   "<item-id> [--size <n>] [--cursor <c>]",
				summary: "list the comments, oldest first",
				run:     commentList,
			},
			{
				name:    "add",
				usage:   "<item-id> --body <text> [--reply-to <comment-id>]",
				summary: "add a comment, or a reply to one",
				run:     commentAdd,
			},
		},
	}
}

// commentsPath is the collection under one entry: comments have no address of their own outside
// the entry they discuss.
func commentsPath(item string) string { return itemsPath + "/" + item + "/comments" }

func commentList(ctx context.Context, cli *CLI, args []string) error {
	const usage = "comment ls <item-id> [--size <n>] [--cursor <c>]"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "comment", "ls", "<item-id> [--size <n>] [--cursor <c>]")
	size := flags.Int("size", 0, "how many comments per page (the server decides when unset)")
	cursor := flags.String("cursor", "", "continue the previous page")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	query := url.Values{}
	if *size > 0 {
		query.Set("size", strconv.Itoa(*size))
	}
	if *cursor != "" {
		query.Set("cursor", *cursor)
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var page openapi.CommentPage
	if err := client.Get(ctx, commentsPath(item.String()), query, &page); err != nil {
		return err
	}

	if err := cli.Emit(page, commentTable(page.Data)); err != nil {
		return err
	}
	cli.reportMore(page.Page)
	return nil
}

func commentAdd(ctx context.Context, cli *CLI, args []string) error {
	const usage = "comment add <item-id> --body <text> [--reply-to <comment-id>]"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "comment", "add", "<item-id> --body <text> [--reply-to <comment-id>]")
	text := flags.String("body", "", "what to say")
	replyTo := flags.String("reply-to", "", "the comment this answers")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	if *text == "" {
		message, _ := cli.Catalogue.Message("comments.body_required", nil)
		return usageError{error: errorString(message)}
	}

	body := openapi.AddCommentJSONBody{Body: *text}
	if *replyTo != "" {
		parent, err := cli.parseID("--reply-to", *replyTo)
		if err != nil {
			return err
		}
		body.ParentCommentId = &parent
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var created openapi.Comment
	if err := client.Post(ctx, commentsPath(item.String()), body, &created); err != nil {
		return err
	}
	return cli.Emit(created, commentTable([]openapi.Comment{created}))
}

func commentTable(comments []openapi.Comment) Table {
	rows := make([][]string, 0, len(comments))
	for _, comment := range comments {
		created := comment.CreatedAt
		rows = append(rows, []string{
			comment.Id.String(),
			comment.AuthorId.String(),
			shortTime(&created),
			id(comment.ParentCommentId),
			oneLine(comment.Body),
		})
	}
	return Table{Columns: []string{"id", "author", "created", "reply-to", "body"}, Rows: rows}
}

// oneLine keeps a comment's body on its row. A newline in a tabwriter cell would start a new row
// with the wrong number of columns; the full text is under --json, where nothing is folded.
func oneLine(body *string) string {
	if body == nil {
		return "(deleted)"
	}
	return strings.Join(strings.Fields(*body), " ")
}
