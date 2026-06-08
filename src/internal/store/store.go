package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/google/uuid"
)

const partitionValue = "todo"

type Todo struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"createdAt"`
	PK        string    `json:"pk"`
}

type Config struct {
	ConnectionString string
	Endpoint         string
	Key              string
	Database         string
	Container        string
	HTTPClient       *http.Client
}

type Store struct {
	container *azcosmos.ContainerClient
}

func New(ctx context.Context, cfg Config) (*Store, error) {
	opts := &azcosmos.ClientOptions{}
	if cfg.HTTPClient != nil {
		opts.ClientOptions = azcore.ClientOptions{
			Transport: cfg.HTTPClient,
			Retry:     policy.RetryOptions{MaxRetries: 5},
		}
	}

	var (
		client *azcosmos.Client
		err    error
	)
	if cfg.ConnectionString != "" {
		client, err = azcosmos.NewClientFromConnectionString(cfg.ConnectionString, opts)
	} else {
		var cred azcosmos.KeyCredential
		cred, err = azcosmos.NewKeyCredential(cfg.Key)
		if err != nil {
			return nil, fmt.Errorf("credential: %w", err)
		}
		client, err = azcosmos.NewClientWithKey(cfg.Endpoint, cred, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("cosmos client: %w", err)
	}

	container, err := client.NewContainer(cfg.Database, cfg.Container)
	if err != nil {
		return nil, fmt.Errorf("container: %w", err)
	}
	return &Store{container: container}, nil
}

func (s *Store) Create(ctx context.Context, title string) (*Todo, error) {
	t := &Todo{
		ID:        uuid.NewString(),
		Title:     title,
		Done:      false,
		CreatedAt: time.Now().UTC(),
		PK:        partitionValue,
	}
	b, _ := json.Marshal(t)
	pk := azcosmos.NewPartitionKeyString(partitionValue)
	if _, err := s.container.CreateItem(ctx, pk, b, nil); err != nil {
		return nil, fmt.Errorf("create item: %w", err)
	}
	return t, nil
}

func (s *Store) Toggle(ctx context.Context, id string) error {
	t, err := s.get(ctx, id)
	if err != nil {
		return err
	}
	t.Done = !t.Done
	b, _ := json.Marshal(t)
	pk := azcosmos.NewPartitionKeyString(partitionValue)
	if _, err := s.container.ReplaceItem(ctx, pk, id, b, nil); err != nil {
		return fmt.Errorf("replace: %w", err)
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	pk := azcosmos.NewPartitionKeyString(partitionValue)
	if _, err := s.container.DeleteItem(ctx, pk, id, nil); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	return nil
}

func (s *Store) List(ctx context.Context) ([]Todo, error) {
	pk := azcosmos.NewPartitionKeyString(partitionValue)
	pager := s.container.NewQueryItemsPager("SELECT * FROM c", pk, nil)
	var todos []Todo
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("query: %w", err)
		}
		for _, item := range page.Items {
			var t Todo
			if err := json.Unmarshal(item, &t); err != nil {
				return nil, err
			}
			todos = append(todos, t)
		}
	}
	sort.Slice(todos, func(i, j int) bool {
		return todos[i].CreatedAt.After(todos[j].CreatedAt)
	})
	return todos, nil
}

func (s *Store) get(ctx context.Context, id string) (*Todo, error) {
	pk := azcosmos.NewPartitionKeyString(partitionValue)
	resp, err := s.container.ReadItem(ctx, pk, id, nil)
	if err != nil {
		return nil, fmt.Errorf("read item: %w", err)
	}
	var t Todo
	if err := json.Unmarshal(resp.Value, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func isConflict(err error) bool {
	var rerr *azcore.ResponseError
	return errors.As(err, &rerr) && rerr.StatusCode == http.StatusConflict
}
