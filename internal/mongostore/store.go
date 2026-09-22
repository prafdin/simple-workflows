package mongostore

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/prafdin/simple-workflows/internal/domain"
)

type Store struct {
	collection *mongo.Collection
}

func New(collection *mongo.Collection) *Store {
	return &Store{collection: collection}
}

type document struct {
	ID          string    `bson:"_id"`
	Name        string    `bson:"name"`
	Image       string    `bson:"image"`
	JobName     string    `bson:"jobName"`
	SubmittedAt time.Time `bson:"submittedAt"`
}

func (s *Store) Save(ctx context.Context, w domain.Workflow) error {
	id := domain.NormalizeName(w.Name)
	doc := document{
		ID:          id,
		Name:        w.Name,
		Image:       w.Image,
		JobName:     w.JobName,
		SubmittedAt: w.SubmittedAt,
	}
	opts := options.Replace().SetUpsert(true)
	_, err := s.collection.ReplaceOne(ctx, bson.M{"_id": id}, doc, opts)
	return err
}

func (s *Store) List(ctx context.Context) ([]domain.Workflow, error) {
	cursor, err := s.collection.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var workflows []domain.Workflow
	for cursor.Next(ctx) {
		var doc document
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		workflows = append(workflows, toDomain(doc))
	}
	return workflows, cursor.Err()
}

func (s *Store) Get(ctx context.Context, name string) (domain.Workflow, error) {
	id := domain.NormalizeName(name)
	var doc document
	err := s.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Workflow{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Workflow{}, err
	}
	return toDomain(doc), nil
}

func toDomain(doc document) domain.Workflow {
	return domain.Workflow{
		Name:        doc.Name,
		Image:       doc.Image,
		JobName:     doc.JobName,
		SubmittedAt: doc.SubmittedAt,
	}
}
