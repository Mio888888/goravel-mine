package modules

import (
	"reflect"
	"testing"

	"github.com/goravel/framework/contracts/database/schema"
)

func TestRegistryMergesMigrationsAfterAnchorWithoutReorderingCore(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id: "alpha",
			migrations: []schema.Migration{
				namedMigration{signature: "202607040001_create_scheduled_task_tables"},
			},
		},
	})

	got := registry.MergeMigrationsAfter([]schema.Migration{
		namedMigration{signature: "202607050001_create_mfa_tables"},
		namedMigration{signature: "202607030002_add_storage_config_id_to_attachment_table"},
		namedMigration{signature: "202607050001_create_mfa_tables"},
	}, "202607030002_add_storage_config_id_to_attachment_table")

	signatures := []string{got[0].Signature(), got[1].Signature(), got[2].Signature(), got[3].Signature()}
	want := []string{
		"202607050001_create_mfa_tables",
		"202607030002_add_storage_config_id_to_attachment_table",
		"202607040001_create_scheduled_task_tables",
		"202607050001_create_mfa_tables",
	}
	if !reflect.DeepEqual(signatures, want) {
		t.Fatalf("merged migration signatures = %#v, want %#v", signatures, want)
	}
}

func TestRegistryAppendsMigrationsWhenAnchorMissing(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id: "alpha",
			migrations: []schema.Migration{
				namedMigration{signature: "202607040001_create_scheduled_task_tables"},
			},
		},
	})

	got := registry.MergeMigrationsAfter([]schema.Migration{
		namedMigration{signature: "202607030002_add_storage_config_id_to_attachment_table"},
	}, "missing_anchor")

	signatures := []string{got[0].Signature(), got[1].Signature()}
	want := []string{
		"202607030002_add_storage_config_id_to_attachment_table",
		"202607040001_create_scheduled_task_tables",
	}
	if !reflect.DeepEqual(signatures, want) {
		t.Fatalf("merged migration signatures = %#v, want %#v", signatures, want)
	}
}

func TestRegistryTenantMigrationsIncludesModuleMigrations(t *testing.T) {
	registry := NewRegistry([]Module{
		tenantMigrationStubModule{
			stubModule: stubModule{id: "alpha"},
			tenantMigrations: []schema.Migration{
				namedMigration{signature: "202607090010_create_alpha_item_table"},
			},
		},
	})

	got := registry.TenantMigrations()

	if len(got) != 1 || got[0].Signature() != "202607090010_create_alpha_item_table" {
		t.Fatalf("TenantMigrations() = %#v, want module migration", got)
	}
}

type tenantMigrationStubModule struct {
	stubModule
	tenantMigrations []schema.Migration
}

func (m tenantMigrationStubModule) TenantMigrations() []schema.Migration {
	return m.tenantMigrations
}
