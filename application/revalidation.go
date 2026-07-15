package application

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/networkteam/sdd/internal/model"
)

// revalidatePreparedTransition proves that the structured facts retained in
// durable intent still form a valid graph against the fresh target snapshot.
// Canonical content is never regenerated during recovery.
func revalidatePreparedTransition(ctx context.Context, snapshot *Snapshot, prepared PreparedTransition) error {
	if snapshot == nil || snapshot.Project() != prepared.Target.Project {
		return &ApplicationError{Code: ErrorRecoveryRequired, Message: "mutation target snapshot does not match prepared target"}
	}
	data := cloneSnapshotData(snapshot.data)
	for _, change := range prepared.Batch.Changes {
		if strings.HasPrefix(filepathSlash(change.LogicalPath), "wip/") {
			applyPreparedWIP(&data, change)
			continue
		}
		if _, err := model.RelPathToID(change.LogicalPath); err != nil {
			return fmt.Errorf("sdd: prepared mutation path %q is neither an entry nor WIP marker: %w", change.LogicalPath, err)
		}
		if change.Delete {
			removeEntryDocument(&data, change.LogicalPath)
			continue
		}
		if change.Document == nil {
			return &ApplicationError{Code: ErrorMigrationRequired, Message: "prepared entry mutation lacks structured document facts", Version: prepared.Version}
		}
		parsed, err := parseEntryDocument(change.LogicalPath, change.CanonicalBytes)
		if err != nil {
			return fmt.Errorf("sdd: validating prepared canonical entry %q: %w", change.LogicalPath, err)
		}
		if !reflect.DeepEqual(parsed, *change.Document) {
			return &ApplicationError{Code: ErrorRecoveryRequired, Message: "prepared structured entry and canonical bytes diverge"}
		}
		upsertEntryDocument(&data, parsed)
	}
	data.Revision = "prepared-revalidation"
	if _, err := BuildSnapshot(ctx, data); err != nil {
		return fmt.Errorf("sdd: prepared mutation no longer validates against target: %w", err)
	}
	return nil
}

func applyPreparedWIP(data *SnapshotData, change DocumentChange) {
	for index := range data.WIP {
		if data.WIP[index].LogicalPath != change.LogicalPath {
			continue
		}
		if change.Delete {
			data.WIP = append(data.WIP[:index], data.WIP[index+1:]...)
		} else {
			data.WIP[index].Content = string(change.CanonicalBytes)
		}
		return
	}
	if !change.Delete {
		data.WIP = append(data.WIP, WIPDocument{LogicalPath: change.LogicalPath, Content: string(change.CanonicalBytes)})
	}
}

func removeEntryDocument(data *SnapshotData, logicalPath string) {
	for index := range data.Entries {
		if data.Entries[index].LogicalPath == logicalPath {
			data.Entries = append(data.Entries[:index], data.Entries[index+1:]...)
			return
		}
	}
}

func upsertEntryDocument(data *SnapshotData, document EntryDocument) {
	for index := range data.Entries {
		if data.Entries[index].LogicalPath == document.LogicalPath {
			data.Entries[index] = document
			return
		}
	}
	data.Entries = append(data.Entries, document)
}

func filepathSlash(path string) string { return strings.ReplaceAll(path, "\\", "/") }
