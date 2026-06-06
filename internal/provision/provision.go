package provision

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/Azure/azure-sdk-for-go/sdk/data/aztables"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azqueue"

	"github.com/DanielTangnes/azlocal/internal/config"
)

// Retry budget for the first call against each emulator, to absorb the gap
// between "container healthy" and "service accepting requests".
const (
	retryAttempts = 15
	retryBackoff  = 2 * time.Second
)

// logf prints an indented progress line.
func logf(format string, args ...any) {
	fmt.Printf("  "+format+"\n", args...)
}

// CreateResources creates every resource declared in cfg. Storage and Cosmos
// resources are created over the wire; Service Bus entities are provisioned
// declaratively via the mounted Config.json, so nothing is done for them here.
// All creates are idempotent.
func CreateResources(ctx context.Context, cfg *config.Config) error {
	s := cfg.Services
	if s.Blob != nil || s.Queue != nil || s.Table != nil {
		if err := createStorage(ctx, cfg); err != nil {
			return err
		}
	}
	if s.Cosmos != nil {
		if err := createCosmos(ctx, cfg); err != nil {
			return err
		}
	}
	return nil
}

func createStorage(ctx context.Context, cfg *config.Config) error {
	conn := storageConnString(cfg)
	s := cfg.Services

	if s.Blob != nil {
		client, err := azblob.NewClientFromConnectionString(conn, nil)
		if err != nil {
			return fmt.Errorf("blob client: %w", err)
		}
		for _, name := range s.Blob.Containers {
			if err := withRetry(ctx, func() error {
				_, err := client.CreateContainer(ctx, name, nil)
				return err
			}); err != nil {
				return fmt.Errorf("create blob container %q: %w", name, err)
			}
			logf("blob container %q ready", name)
		}
	}

	if s.Queue != nil {
		svc, err := azqueue.NewServiceClientFromConnectionString(conn, nil)
		if err != nil {
			return fmt.Errorf("queue client: %w", err)
		}
		for _, name := range s.Queue.Queues {
			qc := svc.NewQueueClient(name)
			if err := withRetry(ctx, func() error {
				_, err := qc.Create(ctx, nil)
				return err
			}); err != nil {
				return fmt.Errorf("create queue %q: %w", name, err)
			}
			logf("queue %q ready", name)
		}
	}

	if s.Table != nil {
		svc, err := aztables.NewServiceClientFromConnectionString(conn, nil)
		if err != nil {
			return fmt.Errorf("table client: %w", err)
		}
		for _, name := range s.Table.Tables {
			if err := withRetry(ctx, func() error {
				_, err := svc.CreateTable(ctx, name, nil)
				return err
			}); err != nil {
				return fmt.Errorf("create table %q: %w", name, err)
			}
			logf("table %q ready", name)
		}
	}
	return nil
}

func createCosmos(ctx context.Context, cfg *config.Config) error {
	client, err := cosmosClient(cfg)
	if err != nil {
		return err
	}
	for _, db := range cfg.Services.Cosmos.Databases {
		if err := withRetry(ctx, func() error {
			_, err := client.CreateDatabase(ctx, azcosmos.DatabaseProperties{ID: db.Name}, nil)
			return err
		}); err != nil {
			return fmt.Errorf("create cosmos database %q: %w", db.Name, err)
		}
		logf("cosmos database %q ready", db.Name)

		dbc, err := client.NewDatabase(db.Name)
		if err != nil {
			return err
		}
		for _, con := range db.Containers {
			c := con
			if err := withRetry(ctx, func() error {
				_, err := dbc.CreateContainer(ctx, azcosmos.ContainerProperties{
					ID:                     c.Name,
					PartitionKeyDefinition: azcosmos.PartitionKeyDefinition{Paths: []string{c.PartitionKey}},
				}, nil)
				return err
			}); err != nil {
				return fmt.Errorf("create cosmos container %q: %w", c.Name, err)
			}
			logf("cosmos container %q ready", c.Name)
		}
	}
	return nil
}

// cosmosClient builds a Cosmos client that trusts the emulator's TLS cert.
func cosmosClient(cfg *config.Config) (*azcosmos.Client, error) {
	cred, err := azcosmos.NewKeyCredential(cosmosKey)
	if err != nil {
		return nil, err
	}
	return azcosmos.NewClientWithKey(cosmosEndpoint(cfg), cred, &azcosmos.ClientOptions{
		ClientOptions: azcore.ClientOptions{Transport: insecureHTTPClient()},
	})
}

// withRetry runs fn, treating an "already exists" conflict as success and
// retrying any other error with a fixed backoff (the emulator may not be ready
// to serve requests the instant its container reports healthy).
func withRetry(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; attempt < retryAttempts; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if isConflict(err) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryBackoff):
		}
	}
	return err
}

// isConflict reports whether err is a 409 (resource already exists).
func isConflict(err error) bool {
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		return respErr.StatusCode == http.StatusConflict
	}
	return false
}
