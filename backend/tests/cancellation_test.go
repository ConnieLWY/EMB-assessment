package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCancelReservationReleasesSlot(t *testing.T) {
	f := newFixture(t)
	start := time.Now().UTC().Add(3 * time.Hour).Truncate(time.Second)
	create := request(f.router, http.MethodPost, "/api/chargers/charger-1/reserve", booking(f.userID, start, start.Add(time.Hour)), f.cookie)
	if create.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", create.Code, create.Body.String())
	}
	var created struct {
		Reservation struct {
			ID string `json:"id"`
		} `json:"reservation"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	path := "/api/reservations/" + created.Reservation.ID + "/cancel"
	cancel := request(f.router, http.MethodPost, path, "", f.cookie)
	if cancel.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", cancel.Code, cancel.Body.String())
	}
	var response struct {
		Reservation struct {
			Status string `json:"status"`
		} `json:"reservation"`
	}
	if err := json.Unmarshal(cancel.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Reservation.Status != "CANCELLED" {
		t.Fatalf("status=%q", response.Reservation.Status)
	}
	rebook := request(f.router, http.MethodPost, "/api/chargers/charger-1/reserve", booking(f.userID, start, start.Add(time.Hour)), f.cookie)
	if rebook.Code != http.StatusCreated {
		t.Fatalf("rebook: %d %s", rebook.Code, rebook.Body.String())
	}
	list := request(f.router, http.MethodGet, "/api/reservations", "", f.cookie)
	var listed struct {
		Reservations []struct {
			Status string `json:"status"`
		} `json:"reservations"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if list.Code != http.StatusOK || len(listed.Reservations) != 2 {
		t.Fatalf("list: %d %s", list.Code, list.Body.String())
	}
}

func TestCancelReservationEnforcesOwnerAndState(t *testing.T) {
	f := newFixture(t)
	start := time.Now().UTC().Add(3 * time.Hour).Truncate(time.Second)
	create := request(f.router, http.MethodPost, "/api/chargers/charger-1/reserve", booking(f.userID, start, start.Add(time.Hour)), f.cookie)
	if create.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", create.Code, create.Body.String())
	}
	var created struct {
		Reservation struct {
			ID string `json:"id"`
		} `json:"reservation"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	path := "/api/reservations/" + created.Reservation.ID + "/cancel"
	other := request(f.router, http.MethodPost, "/api/auth/login", `{"username":"demo2","password":"DemoPass123!"}`, nil)
	if other.Code != http.StatusOK {
		t.Fatalf("other login: %d %s", other.Code, other.Body.String())
	}
	if rec := request(f.router, http.MethodPost, path, "", other.Result().Cookies()[0]); rec.Code != http.StatusNotFound || errorCode(t, rec) != "RESERVATION_NOT_FOUND" {
		t.Fatalf("other user: %d %s", rec.Code, rec.Body.String())
	}
	if rec := request(f.router, http.MethodPost, path, "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: %d %s", rec.Code, rec.Body.String())
	}
	if rec := request(f.router, http.MethodPost, "/api/reservations/not-a-uuid/cancel", "", f.cookie); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "VALIDATION_ERROR" || !strings.Contains(rec.Body.String(), "valid reservation ID") {
		t.Fatalf("invalid ID: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := f.pool.Exec(context.Background(), "UPDATE reservations SET status='ACTIVE' WHERE id=$1", created.Reservation.ID); err != nil {
		t.Fatal(err)
	}
	if rec := request(f.router, http.MethodPost, path, "", f.cookie); rec.Code != http.StatusConflict || errorCode(t, rec) != "RESERVATION_NOT_CANCELLABLE" {
		t.Fatalf("active: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := f.pool.Exec(context.Background(), "UPDATE reservations SET status='WAITING' WHERE id=$1", created.Reservation.ID); err != nil {
		t.Fatal(err)
	}
	if rec := request(f.router, http.MethodPost, path, "", f.cookie); rec.Code != http.StatusOK {
		t.Fatalf("waiting: %d %s", rec.Code, rec.Body.String())
	}
	if rec := request(f.router, http.MethodPost, path, "", f.cookie); rec.Code != http.StatusConflict || errorCode(t, rec) != "RESERVATION_NOT_CANCELLABLE" {
		t.Fatalf("repeated cancel: %d %s", rec.Code, rec.Body.String())
	}
}
