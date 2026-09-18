package worker

import (
	"context"
	"testing"
	"time"

	"github.com/Silo-Server/silo-server/internal/models"
)

type onDemandPersonRefresher struct {
	refreshed  chan int64
	discovered int
}

func (r *onDemandPersonRefresher) RefreshPerson(_ context.Context, id int64) (*models.Person, error) {
	r.refreshed <- id
	return &models.Person{ID: id}, nil
}

// Retained on the fake to catch accidental catalogue discovery.
func (r *onDemandPersonRefresher) FindCandidates(context.Context, int) ([]int64, error) {
	r.discovered++
	return []int64{99}, nil
}

func TestPersonRefreshWorkerDoesNotDiscoverPeople(t *testing.T) {
	service := &onDemandPersonRefresher{refreshed: make(chan int64, 10)}
	w := NewPersonRefreshWorker(service, DefaultPersonRefreshWorkerConfig())
	w.processBatch()
	if service.discovered != 0 || len(service.refreshed) != 0 {
		t.Fatal("idle worker discovered or refreshed unrequested people")
	}
	w.Enqueue(7)
	w.Enqueue(7)
	w.Enqueue(0)
	w.processBatch()
	if service.discovered != 0 || len(service.refreshed) != 1 {
		t.Fatal("requested batch must not discover extra people or duplicate requests")
	}
	if got := <-service.refreshed; got != 7 {
		t.Fatalf("refreshed %d, want 7", got)
	}
}

func TestPersonRefreshWorkerDrainsRequestsAcrossBatchLimit(t *testing.T) {
	service := &onDemandPersonRefresher{refreshed: make(chan int64, 10)}
	w := NewPersonRefreshWorker(service, PersonRefreshWorkerConfig{BatchSize: 2})
	for id := int64(1); id <= 5; id++ {
		w.Enqueue(id)
	}
	w.Start()
	t.Cleanup(w.Stop)
	for want := int64(1); want <= 5; want++ {
		select {
		case got := <-service.refreshed:
			if got != want {
				t.Fatalf("refreshed %d, want %d", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("request %d was stranded after a full batch", want)
		}
	}
}
