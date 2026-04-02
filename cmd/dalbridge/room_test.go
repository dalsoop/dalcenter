package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupRoomTestServer() (*http.ServeMux, *roomRegistry, *deliveryTracker) {
	rr := newRoomRegistry()
	dt := newDeliveryTracker()
	mux := http.NewServeMux()
	registerRoomHandlers(mux, rr, dt)
	registerDeliveryHandlers(mux, dt)
	return mux, rr, dt
}

func TestCreateRoom(t *testing.T) {
	mux, _, _ := setupRoomTestServer()

	body := `{"name":"team-alpha","channel":"alpha-ch","webhook":"http://localhost:9000/api/message"}`
	req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var rm room
	if err := json.NewDecoder(w.Body).Decode(&rm); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if rm.Name != "team-alpha" {
		t.Errorf("name = %q, want %q", rm.Name, "team-alpha")
	}
	if rm.Channel != "alpha-ch" {
		t.Errorf("channel = %q, want %q", rm.Channel, "alpha-ch")
	}
}

func TestCreateRoomDuplicate(t *testing.T) {
	mux, _, _ := setupRoomTestServer()

	body := `{"name":"dup-room","channel":"ch"}`
	req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d", w.Code)
	}

	// duplicate
	req = httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate: expected 409, got %d", w.Code)
	}
}

func TestCreateRoomDefaultChannel(t *testing.T) {
	mux, _, _ := setupRoomTestServer()

	body := `{"name":"no-channel"}`
	req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var rm room
	json.NewDecoder(w.Body).Decode(&rm)
	if rm.Channel != "no-channel" {
		t.Errorf("channel should default to name, got %q", rm.Channel)
	}
}

func TestCreateRoomMissingName(t *testing.T) {
	mux, _, _ := setupRoomTestServer()

	body := `{"channel":"ch"}`
	req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestListRooms(t *testing.T) {
	mux, _, _ := setupRoomTestServer()

	// create two rooms
	for _, name := range []string{"room-a", "room-b"} {
		body := `{"name":"` + name + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s: expected 201, got %d", name, w.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/rooms", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}

	var rooms []room
	json.NewDecoder(w.Body).Decode(&rooms)
	if len(rooms) != 2 {
		t.Errorf("expected 2 rooms, got %d", len(rooms))
	}
}

func TestGetRoom(t *testing.T) {
	mux, _, _ := setupRoomTestServer()

	// create
	body := `{"name":"get-me","channel":"get-ch"}`
	req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// get
	req = httptest.NewRequest(http.MethodGet, "/api/rooms/get-me", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", w.Code)
	}
	var rm room
	json.NewDecoder(w.Body).Decode(&rm)
	if rm.Name != "get-me" {
		t.Errorf("name = %q, want %q", rm.Name, "get-me")
	}
}

func TestGetRoomNotFound(t *testing.T) {
	mux, _, _ := setupRoomTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/rooms/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteRoom(t *testing.T) {
	mux, _, _ := setupRoomTestServer()

	// create
	body := `{"name":"del-me"}`
	req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// delete
	req = httptest.NewRequest(http.MethodDelete, "/api/rooms/del-me", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d", w.Code)
	}

	// verify gone
	req = httptest.NewRequest(http.MethodGet, "/api/rooms/del-me", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("after delete: expected 404, got %d", w.Code)
	}
}

func TestDeleteRoomNotFound(t *testing.T) {
	mux, _, _ := setupRoomTestServer()

	req := httptest.NewRequest(http.MethodDelete, "/api/rooms/ghost", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestRoomSend(t *testing.T) {
	mux, _, _ := setupRoomTestServer()

	// create room
	body := `{"name":"send-test","channel":"ch-send"}`
	req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}

	// send message
	body = `{"text":"hello room","username":"bot"}`
	req = httptest.NewRequest(http.MethodPost, "/api/rooms/send-test/send", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("send: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// verify response has delivery_id
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["delivery_id"] == "" {
		t.Error("response should contain delivery_id")
	}
}

func TestRoomSendDeliveryTracking(t *testing.T) {
	mux, _, _ := setupRoomTestServer()

	// create room (no webhook — SSE only)
	body := `{"name":"track-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// send message
	body = `{"text":"tracked msg","username":"bot"}`
	req = httptest.NewRequest(http.MethodPost, "/api/rooms/track-test/send", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	deliveryID := resp["delivery_id"]

	// check delivery status
	req = httptest.NewRequest(http.MethodGet, "/api/deliveries/"+deliveryID, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("delivery get: expected 200, got %d", w.Code)
	}

	var d delivery
	json.NewDecoder(w.Body).Decode(&d)
	if d.Status != statusDelivered {
		t.Errorf("delivery status = %q, want %q", d.Status, statusDelivered)
	}
	if d.Room != "track-test" {
		t.Errorf("delivery room = %q, want %q", d.Room, "track-test")
	}
}

func TestDeliveryListByRoom(t *testing.T) {
	mux, _, _ := setupRoomTestServer()

	// create room
	body := `{"name":"list-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/rooms", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// send two messages
	for i := 0; i < 2; i++ {
		body = `{"text":"msg","username":"bot"}`
		req = httptest.NewRequest(http.MethodPost, "/api/rooms/list-test/send", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, req)
	}

	// list deliveries for room
	req = httptest.NewRequest(http.MethodGet, "/api/deliveries?room=list-test", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}

	var items []delivery
	json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 2 {
		t.Errorf("expected 2 deliveries, got %d", len(items))
	}
}

func TestRoomSendNotFound(t *testing.T) {
	mux, _, _ := setupRoomTestServer()

	body := `{"text":"hello","username":"bot"}`
	req := httptest.NewRequest(http.MethodPost, "/api/rooms/no-room/send", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestRoomStreamNotFound(t *testing.T) {
	mux, _, _ := setupRoomTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/rooms/no-room/stream", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestUnknownAction(t *testing.T) {
	mux, _, _ := setupRoomTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/rooms/any/unknown", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
