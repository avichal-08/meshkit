package registration

import (
	"testing"

	"github.com/gofrs/uuid"
	"github.com/meshery/meshkit/database"
	"github.com/meshery/meshkit/models/meshmodel/entity"
	meshmodel "github.com/meshery/meshkit/models/meshmodel/registry"
	regv1alpha3 "github.com/meshery/meshkit/models/meshmodel/registry/v1alpha3"
	"github.com/meshery/schemas/models/v1alpha3"
	"github.com/meshery/schemas/models/v1alpha3/relationship"
	"github.com/meshery/schemas/models/v1beta1"
	"github.com/meshery/schemas/models/v1beta1/category"
	"github.com/meshery/schemas/models/v1beta1/connection"
	"github.com/meshery/schemas/models/v1beta1/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingErrorStore is a RegistrationErrorStore that keeps every reported error.
type recordingErrorStore struct {
	errs []error
}

func (s *recordingErrorStore) AddInvalidDefinition(_ string, err error) {
	s.errs = append(s.errs, err)
}

func (s *recordingErrorStore) InsertEntityRegError(_ string, _ string, _ entity.EntityType, _ string, err error) {
	s.errs = append(s.errs, err)
}

// packagingUnitWithRelationships returns a PackagingUnit for the same model
// on every call, carrying relationships with the given subTypes. Each call
// gives the model a fresh random ID, as a model read from a model file
// carries an ID unrelated to the one the registry stores it under.
func packagingUnitWithRelationships(subTypes ...string) PackagingUnit {
	rels := make([]relationship.RelationshipDefinition, 0, len(subTypes))
	for _, subType := range subTypes {
		rels = append(rels, relationship.RelationshipDefinition{
			SchemaVersion:    v1alpha3.RelationshipSchemaVersion,
			Version:          "v1.0.0",
			Kind:             relationship.Edge,
			RelationshipType: "binding",
			SubType:          subType,
		})
	}

	return PackagingUnit{
		Model: model.ModelDefinition{
			ID:            uuid.Must(uuid.NewV4()),
			SchemaVersion: v1beta1.ModelSchemaVersion,
			Version:       "v1.0.0",
			Name:          "test-model",
			DisplayName:   "Test Model",
			Status:        model.Enabled,
			Registrant: connection.Connection{
				Name:   "github",
				Kind:   "github",
				Type:   "registry",
				Status: connection.ConnectionStatusRegistered,
			},
			Category: category.CategoryDefinition{Name: "test-category"},
			Model:    model.Model{Version: "v1.0.0"},
		},
		Relationships: rels,
	}
}

// TestRegister_ReimportedModelKeepsRelationships is a regression test for
// meshery/meshery#22424: re-importing an already-registered model must link
// its relationships to the stored model, so every one of them is returned by
// the registry. Previously they were saved against the model file's ID and
// dropped by the join on model_dbs without any error.
func TestRegister_ReimportedModelKeepsRelationships(t *testing.T) {
	db, err := database.New(database.Options{
		Engine:   database.SQLITE,
		Filename: ":memory:",
	})
	require.NoError(t, err)

	rm, err := meshmodel.NewRegistryManager(&db)
	require.NoError(t, err)
	t.Cleanup(func() {
		rm.Cleanup()
		assert.NoError(t, db.DBClose())
	})

	errStore := &recordingErrorStore{}
	rh := NewRegistrationHelper(t.TempDir(), rm, errStore)

	rh.register(packagingUnitWithRelationships("first", "second"))
	rh.register(packagingUnitWithRelationships("first", "second", "third"))
	require.Empty(t, errStore.errs)

	for _, subType := range []string{"first", "second", "third"} {
		t.Run(subType, func(t *testing.T) {
			filter := &regv1alpha3.RelationshipFilter{ModelName: "test-model", SubType: subType}
			_, count, _, err := filter.Get(&db)
			require.NoError(t, err)
			assert.NotZero(t, count, "relationship %q of the re-imported model must be registered", subType)
		})
	}
}
