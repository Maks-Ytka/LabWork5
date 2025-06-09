package main

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestBalancer_WithStubs(t *testing.T) {
	t.Run("Predefined", func(t *testing.T) {
		loads := []int{80, 50, 30, 10, 100, 40, 90, 25, 60, 15}
		runStubbedBalancer(t, loads, "Predefined")
	})

	t.Run("Random", func(t *testing.T) {
		rand.Seed(time.Now().UnixNano())
		loads := make([]int, 10)
		for i := range loads {
			loads[i] = rand.Intn(100) + 1
		}
		runStubbedBalancer(t, loads, "Random")
	})
}

func runStubbedBalancer(t *testing.T, loads []int, label string) {
	if len(loads) != 10 {
		t.Fatal("loads must have length 10")
	}

	health = func(_ string) bool {
		return true
	}

	var muLoad sync.Mutex
	var stubIdx int
	forward = func(dst string, rw http.ResponseWriter, _ *http.Request) error {
		muLoad.Lock()
		n := loads[stubIdx]
		stubIdx++
		muLoad.Unlock()

		rw.Header().Set("lb-from", dst)
		rw.WriteHeader(http.StatusOK)
		buf := make([]byte, n)
		for i := 0; i < n; i++ {
			buf[i] = 'x'
		}
		_, _ = rw.Write(buf)
		return nil
	}

	traffic := make(map[string]uint64, len(serversPool))
	var mu sync.Mutex
	observedSeq := make([]string, 0, len(loads))

	handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		healthy := make([]string, 0, len(serversPool))
		for _, srv := range serversPool {
			if health(srv) {
				healthy = append(healthy, srv)
			}
		}
		if len(healthy) == 0 {
			http.Error(rw, "no healthy backends", http.StatusServiceUnavailable)
			return
		}

		mu.Lock()
		chosen := healthy[0]
		min := traffic[chosen]
		for _, srv := range healthy[1:] {
			if traffic[srv] < min {
				min, chosen = traffic[srv], srv
			}
		}
		mu.Unlock()

		rec := httptest.NewRecorder()
		_ = forward(chosen, rec, r)

		for k, vs := range rec.Header() {
			for _, v := range vs {
				rw.Header().Add(k, v)
			}
		}
		rw.WriteHeader(rec.Code)
		written, _ := io.Copy(rw, rec.Result().Body)

		mu.Lock()
		traffic[chosen] += uint64(written)
		mu.Unlock()

		observedSeq = append(observedSeq, chosen)
	})

	lb := httptest.NewServer(handler)
	defer lb.Close()

	client := &http.Client{Timeout: time.Second}

	mirror := make(map[string]uint64, len(serversPool))
	for _, s := range serversPool {
		mirror[s] = 0
	}

	fmt.Printf("Server Addresses: [S1: %s, S2: %s, S3: %s]\n",
		serversPool[0], serversPool[1], serversPool[2])
	fmt.Printf("No Requests yet, Servers State: [ S1:0, S2:0, S3:0 ]\n")

	for i := range loads {
		reqNum := i + 1
		t.Run(fmt.Sprintf("%s Request %d", label, reqNum), func(t *testing.T) {
			resp, err := client.Get(lb.URL + "/api/v1/some-data")
			if err != nil {
				t.Fatalf("Fetch failed: %v", err)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			n := len(body)
			chosen := resp.Header.Get("lb-from")
			mirror[chosen] += uint64(n)

			var labelIndex string
			for idx, addr := range serversPool {
				if addr == chosen {
					labelIndex = fmt.Sprintf("S%d", idx+1)
				}
			}

			fmt.Printf("%s Request %d, Chosen Server: %s | %s, Servers State: [ S1:%d, S2:%d, S3:%d ]\n",
				label,
				reqNum,
				labelIndex, chosen,
				mirror[serversPool[0]],
				mirror[serversPool[1]],
				mirror[serversPool[2]],
			)
		})
	}

	expected := expectedSequence(loads, serversPool)
	if len(observedSeq) != len(expected) {
		t.Fatalf("observed length %d, expected %d", len(observedSeq), len(expected))
	}
	for i := range expected {
		if observedSeq[i] != expected[i] {
			t.Fatalf("at request %d: got %s, want %s", i+1, observedSeq[i], expected[i])
		}
	}
}

func expectedSequence(loads []int, servers []string) []string {
	traffic := make(map[string]uint64, len(servers))
	seq := make([]string, 0, len(loads))
	for _, s := range servers {
		traffic[s] = 0
	}

	for _, load := range loads {
		chosen := servers[0]
		min := traffic[chosen]
		for _, s := range servers[1:] {
			if traffic[s] < min {
				min, chosen = traffic[s], s
			}
		}
		seq = append(seq, chosen)
		traffic[chosen] += uint64(load)
	}
	return seq
}
