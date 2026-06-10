package provision

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/Azure/azure-sdk-for-go/sdk/data/aztables"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azqueue"

	"github.com/DanielTangnes/azlocal/internal/config"
)

// Seed loads every seed entry in cfg into the running emulators. Target
// syntax and arity are validated by config.ParseSeedTarget (also run at
// config load time, so errors surface before any emulator is started).
func Seed(ctx context.Context, cfg *config.Config) error {
	for _, s := range cfg.Seed {
		t, err := config.ParseSeedTarget(s.Target)
		if err != nil {
			return err
		}

		switch t.Scheme {
		case "blob":
			err = seedBlob(ctx, cfg, t.Parts[0], s.From)
		case "queue":
			err = seedQueue(ctx, cfg, t.Parts[0], s.From)
		case "table":
			err = seedTable(ctx, cfg, t.Parts[0], s.From)
		case "cosmos":
			err = seedCosmos(ctx, cfg, t.Parts[0], t.Parts[1], s.From)
		case "servicebus":
			err = seedServiceBus(ctx, cfg, t.Parts[0], s.From)
		}

		if err != nil {
			return fmt.Errorf("seed %q from %q: %w", s.Target, s.From, err)
		}
	}
	return nil
}

func seedBlob(ctx context.Context, cfg *config.Config, container, from string) error {
	client, err := azblob.NewClientFromConnectionString(StorageConnString(cfg), nil)
	if err != nil {
		return err
	}
	upload := func(path, blobName string) error {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := client.UploadFile(ctx, container, blobName, f, nil); err != nil {
			return fmt.Errorf("upload %q: %w", blobName, err)
		}
		logf("blob %q/%q uploaded", container, blobName)
		return nil
	}

	info, err := os.Stat(from)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return upload(from, filepath.Base(from))
	}
	return filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(from, p)
		if err != nil {
			return err
		}
		return upload(p, filepath.ToSlash(rel))
	})
}

func seedQueue(ctx context.Context, cfg *config.Config, queue, from string) error {
	svc, err := azqueue.NewServiceClientFromConnectionString(StorageConnString(cfg), nil)
	if err != nil {
		return err
	}
	qc := svc.NewQueueClient(queue)
	msgs, err := readMessages(from)
	if err != nil {
		return err
	}
	for _, m := range msgs {
		if _, err := qc.EnqueueMessage(ctx, m, nil); err != nil {
			return err
		}
	}
	logf("queue %q seeded with %d message(s)", queue, len(msgs))
	return nil
}

func seedTable(ctx context.Context, cfg *config.Config, table, from string) error {
	svc, err := aztables.NewServiceClientFromConnectionString(StorageConnString(cfg), nil)
	if err != nil {
		return err
	}
	client := svc.NewClient(table)
	entities, err := readJSONArray(from)
	if err != nil {
		return err
	}
	for i, e := range entities {
		if _, err := client.UpsertEntity(ctx, []byte(e), nil); err != nil {
			return fmt.Errorf("upsert entity #%d (needs PartitionKey and RowKey): %w", i, err)
		}
	}
	logf("table %q seeded with %d entit(ies)", table, len(entities))
	return nil
}

func seedCosmos(ctx context.Context, cfg *config.Config, db, container, from string) error {
	pkPath := cosmosPartitionKey(cfg, db, container)
	if pkPath == "" {
		return fmt.Errorf("container %s/%s is not declared under services.cosmos", db, container)
	}
	client, err := CosmosClient(cfg)
	if err != nil {
		return err
	}
	cc, err := client.NewContainer(db, container)
	if err != nil {
		return err
	}
	docs, err := readDocs(from)
	if err != nil {
		return err
	}
	field := strings.TrimPrefix(pkPath, "/")
	for i, doc := range docs {
		pk, err := extractPartitionKey(doc, field)
		if err != nil {
			return fmt.Errorf("document #%d: %w", i, err)
		}
		if _, err := cc.UpsertItem(ctx, pk, []byte(doc), nil); err != nil {
			return fmt.Errorf("upsert document #%d: %w", i, err)
		}
	}
	logf("cosmos %s/%s seeded with %d document(s)", db, container, len(docs))
	return nil
}

func seedServiceBus(ctx context.Context, cfg *config.Config, entity, from string) error {
	client, err := azservicebus.NewClientFromConnectionString(ServiceBusConnString(), nil)
	if err != nil {
		return err
	}
	defer client.Close(ctx)
	sender, err := client.NewSender(entity, nil)
	if err != nil {
		return err
	}
	defer sender.Close(ctx)

	msgs, err := readMessages(from)
	if err != nil {
		return err
	}
	for _, m := range msgs {
		if err := sender.SendMessage(ctx, &azservicebus.Message{Body: []byte(m)}, nil); err != nil {
			return err
		}
	}
	logf("servicebus %q seeded with %d message(s)", entity, len(msgs))
	return nil
}

// cosmosPartitionKey looks up the declared partition key path for a container.
func cosmosPartitionKey(cfg *config.Config, db, container string) string {
	if cfg.Services.Cosmos == nil {
		return ""
	}
	for _, d := range cfg.Services.Cosmos.Databases {
		if d.Name != db {
			continue
		}
		for _, c := range d.Containers {
			if c.Name == container {
				return c.PartitionKey
			}
		}
	}
	return ""
}

// extractPartitionKey reads the partition key value from a document for the
// given top-level field, inferring its type (string, number, or bool).
func extractPartitionKey(doc json.RawMessage, field string) (azcosmos.PartitionKey, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(doc, &m); err != nil {
		return azcosmos.PartitionKey{}, err
	}
	raw, ok := m[field]
	if !ok {
		return azcosmos.PartitionKey{}, fmt.Errorf("missing partition key field %q", field)
	}
	// JSON null unmarshals into string/number/bool without error, so reject it
	// explicitly rather than silently coercing it to an empty partition key.
	if string(bytes.TrimSpace(raw)) == "null" {
		return azcosmos.PartitionKey{}, fmt.Errorf("partition key field %q is null", field)
	}
	var sv string
	if json.Unmarshal(raw, &sv) == nil {
		return azcosmos.NewPartitionKeyString(sv), nil
	}
	var nv float64
	if json.Unmarshal(raw, &nv) == nil {
		return azcosmos.NewPartitionKeyNumber(nv), nil
	}
	var bv bool
	if json.Unmarshal(raw, &bv) == nil {
		return azcosmos.NewPartitionKeyBool(bv), nil
	}
	return azcosmos.PartitionKey{}, fmt.Errorf("unsupported partition key type for field %q", field)
}

// readMessages reads a JSON array of messages. String elements are used
// verbatim; non-string elements are re-encoded as compact JSON text.
func readMessages(path string) ([]string, error) {
	raw, err := readJSONArray(path)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		var s string
		if json.Unmarshal(r, &s) == nil {
			out = append(out, s)
			continue
		}
		out = append(out, string(r))
	}
	return out, nil
}

// readJSONArray reads a file containing a JSON array, returning its elements.
func readJSONArray(path string) ([]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, fmt.Errorf("expected a JSON array in %s: %w", path, err)
	}
	return arr, nil
}

// readDocs reads documents from either a JSON array or NDJSON (one JSON object
// per line) file.
func readDocs(path string) ([]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return nil, err
		}
		return arr, nil
	}

	var out []json.RawMessage
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		out = append(out, append(json.RawMessage(nil), line...))
	}
	return out, sc.Err()
}
