// Copyright 2022 The Hugoreleaser Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package releases

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gohugoio/hugoreleaser/internal/config"
	"github.com/gohugoio/hugoreleaser/internal/releases/releasetypes"
	"github.com/google/go-github/v45/github"
	"golang.org/x/oauth2"
)

const tokenEnvVar = "GITHUB_TOKEN"

func NewClient(ctx context.Context, typ releasetypes.Type) (Client, error) {
	if typ != releasetypes.GitHub {
		return nil, fmt.Errorf("github: only github is supported for now")
	}
	token := os.Getenv(tokenEnvVar)
	if token == "" {
		return nil, fmt.Errorf("github: missing %q env var", tokenEnvVar)
	}

	// Set in tests to test the all command.
	// We cannot curently use the -try flag because
	// that does not create any archives.
	if token == "faketoken" {
		return &FakeClient{}, nil
	}

	tokenSource := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)

	httpClient := oauth2.NewClient(ctx, tokenSource)

	return &GitHubClient{
		client: github.NewClient(httpClient),
	}, nil
}

// UploadAssetsFileWithRetries is a wrapper around UploadAssetsFile that retries on temporary errors.
func UploadAssetsFileWithRetries(ctx context.Context, client Client, settings config.ReleaseSettings, releaseID int64, openFile func() (*os.File, error)) error {
	return withRetries(func() (error, bool) {
		f, err := openFile()
		if err != nil {
			return err, false
		}
		defer f.Close()
		err = client.UploadAssetsFile(ctx, settings, f, releaseID)
		if err != nil && errors.Is(err, TemporaryError{}) {
			return err, true
		}
		return err, false
	})

}

type Client interface {
	Release(ctx context.Context, tagName, committish string, settings config.ReleaseSettings) (int64, error)
	UploadAssetsFile(ctx context.Context, settings config.ReleaseSettings, f *os.File, releaseID int64) error
}

type GitHubClient struct {
	client *github.Client
}

func (c GitHubClient) Release(ctx context.Context, tagName, committish string, settings config.ReleaseSettings) (int64, error) {
	s := func(s string) *string {
		if s == "" {
			return nil
		}
		return github.String(s)
	}

	var body string

	if settings.ReleaseNotesFilename != "" {
		b, err := os.ReadFile(settings.ReleaseNotesFilename)
		if err != nil {
			return 0, err
		}
		body = string(b)
	}

	r := &github.RepositoryRelease{
		TagName:              s(tagName),
		TargetCommitish:      s(committish),
		Name:                 s(settings.Name),
		Body:                 s(body),
		Draft:                github.Bool(settings.Draft),
		Prerelease:           github.Bool(settings.Prerelease),
		GenerateReleaseNotes: github.Bool(settings.GenerateReleaseNotesOnHost),
	}

	rel, resp, err := c.client.Repositories.CreateRelease(ctx, settings.RepositoryOwner, settings.Repository, r)
	if err != nil {
		return 0, err
	}

	if resp.StatusCode != http.StatusCreated {
		return 0, fmt.Errorf("github: unexpected status code: %d", resp.StatusCode)
	}

	return *rel.ID, nil
}

func (c GitHubClient) UploadAssetsFile(ctx context.Context, settings config.ReleaseSettings, f *os.File, releaseID int64) error {

	_, resp, err := c.client.Repositories.UploadReleaseAsset(
		ctx,
		settings.RepositoryOwner,
		settings.Repository,
		releaseID,
		&github.UploadOptions{
			Name: filepath.Base(f.Name()),
		},
		f,
	)
	if err == nil {
		return nil
	}

	if resp != nil && !isTemporaryHttpStatus(resp.StatusCode) {
		return err
	}

	return TemporaryError{err}

}

type TemporaryError struct {
	error
}

// isTemporaryHttpStatus returns true if the status code is considered temporary, returning
// true if not sure.
func isTemporaryHttpStatus(status int) bool {
	switch status {
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		return false
	default:
		return true
	}
}

// Fake client is only used in tests.
type FakeClient struct {
	releaseID int64
}

func (c *FakeClient) Release(ctx context.Context, tagName string, committish string, settings config.ReleaseSettings) (int64, error) {
	// Tests depen on this string.
	fmt.Printf("fake: release: tag:%s committish:%s %#v\n", tagName, committish, settings)
	c.releaseID = rand.Int63()
	return c.releaseID, nil
}

func (c *FakeClient) UploadAssetsFile(ctx context.Context, settings config.ReleaseSettings, f *os.File, releaseID int64) error {
	if c.releaseID != releaseID {
		return fmt.Errorf("fake: releaseID mismatch: %d != %d", c.releaseID, releaseID)
	}
	if f == nil {
		return fmt.Errorf("fake: nil file")
	}
	return nil
}
