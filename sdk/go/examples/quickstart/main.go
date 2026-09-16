// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command quickstart is the Go SDK's example: list the collections of a hub, create an entry in
// the first one, read it back.
//
//	HUBTASK_URL=https://hubtask.example/api/v1 HUBTASK_TOKEN=hbt_pat_… HUBTASK_HUB=<hub-id> \
//	  go run ./sdk/go/examples/quickstart
//
// Every call is the generated client's; the three helpers it uses beside them - WithBearer,
// Check, the idempotency key - are the whole of what the SDK adds by hand.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/Jersyfi/hubtask/sdk/go/hubtask"
)

func main() {
	if err := run(); err != nil {
		var problem *hubtask.ProblemError
		if errors.As(err, &problem) {
			fmt.Fprintf(os.Stderr, "refused: %v (request %v)\n", err, deref(problem.Problem.RequestId))
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func run() error {
	base, token, hub := os.Getenv("HUBTASK_URL"), os.Getenv("HUBTASK_TOKEN"), os.Getenv("HUBTASK_HUB")
	if base == "" || token == "" || hub == "" {
		return errors.New("set HUBTASK_URL, HUBTASK_TOKEN and HUBTASK_HUB")
	}
	hubID, err := uuid.Parse(hub)
	if err != nil {
		return fmt.Errorf("HUBTASK_HUB: %w", err)
	}

	client, err := hubtask.NewClientWithResponses(base, hubtask.WithRequestEditorFn(hubtask.WithBearer(token)))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The collections under the hub: a page, walked once.
	kind := hubtask.ContainerTypeFilter("COLLECTION")
	collections, err := client.ListContainersWithResponse(ctx, &hubtask.ListContainersParams{Type: &kind, ParentId: &hubID})
	if err := hubtask.Check(err, collections); err != nil {
		return err
	}
	if len(collections.JSON200.Data) == 0 {
		return errors.New("the hub has no collection to create in")
	}
	for _, collection := range collections.JSON200.Data {
		fmt.Printf("%s  %s\n", collection.Id, collection.Name)
	}

	// An entry in the first one. The idempotency key is what makes running this twice after a
	// lost connection safe: the second run with the same key is answered, not applied.
	first := collections.JSON200.Data[0].Id
	key := uuid.New()
	created, err := client.CreateWorkItemWithResponse(ctx,
		&hubtask.CreateWorkItemParams{IdempotencyKey: &key},
		hubtask.CreateWorkItemJSONRequestBody{CollectionId: &first, Type: "TASK", Title: "Made by the Go SDK"})
	if err := hubtask.Check(err, created); err != nil {
		return err
	}
	fmt.Printf("created %s (version %d)\n", created.JSON201.Id, created.JSON201.Version)

	// Read it back, which is what a client does after every write it did not get an ETag from.
	read, err := client.GetWorkItemWithResponse(ctx, created.JSON201.Id, &hubtask.GetWorkItemParams{})
	if err := hubtask.Check(err, read); err != nil {
		return err
	}
	fmt.Printf("read %q\n", read.JSON200.Title)
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
