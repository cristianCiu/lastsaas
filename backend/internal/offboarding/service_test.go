package offboarding

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestPseudonymIsDeterministicOpaqueObjectID(t *testing.T) {
	tenant := primitive.NewObjectID()
	actor := primitive.NewObjectID()
	first := pseudonym(tenant, actor)
	if first != pseudonym(tenant, actor) {
		t.Fatal("pseudonym must be stable across retries")
	}
	id, err := primitive.ObjectIDFromHex(first)
	if err != nil || id.IsZero() || id == actor {
		t.Fatalf("pseudonym must be a non-original ObjectID: %q", first)
	}
	if first == pseudonym(primitive.NewObjectID(), actor) {
		t.Fatal("pseudonym must be tenant isolated")
	}
}
