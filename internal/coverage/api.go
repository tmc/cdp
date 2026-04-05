package coverage

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// StartAPI starts an HTTP server exposing coverage data. It serves on the
// given port and never returns (intended to be called in a goroutine).
func StartAPI(port int, store Store) {
	if port <= 0 {
		return
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/coverage/snapshots", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w)
		if store == nil {
			json.NewEncoder(w).Encode([]any{})
			return
		}
		snaps := store.Snapshots()
		type snapInfo struct {
			Name            string  `json:"name"`
			Timestamp       string  `json:"timestamp"`
			Files           int     `json:"files"`
			CoveragePercent float64 `json:"coverage_percent"`
		}
		var out []snapInfo
		for _, s := range snaps {
			summary := s.Summary()
			totalLines, coveredLines := 0, 0
			for _, fs := range summary {
				totalLines += fs.TotalLines
				coveredLines += fs.CoveredLines
			}
			pct := 0.0
			if totalLines > 0 {
				pct = float64(coveredLines) / float64(totalLines) * 100
			}
			out = append(out, snapInfo{
				Name:            s.Name,
				Timestamp:       s.Timestamp.Format("2006-01-02T15:04:05Z"),
				Files:           len(summary),
				CoveragePercent: pct,
			})
		}
		if out == nil {
			out = []snapInfo{}
		}
		json.NewEncoder(w).Encode(out)
	})

	mux.HandleFunc("GET /api/coverage/snapshot/{name}", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w)
		name := r.PathValue("name")
		snap := findSnap(store, name)
		if snap == nil {
			http.Error(w, "snapshot not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(snap.Summary())
	})

	mux.HandleFunc("GET /api/coverage/delta", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w)
		before := r.URL.Query().Get("before")
		after := r.URL.Query().Get("after")
		if store == nil {
			http.Error(w, "no coverage data", http.StatusNotFound)
			return
		}
		snapBefore := findSnap(store, before)
		snapAfter := findSnap(store, after)
		if snapBefore == nil || snapAfter == nil {
			http.Error(w, "snapshot not found", http.StatusNotFound)
			return
		}
		delta := store.ComputeDelta(snapBefore, snapAfter)
		json.NewEncoder(w).Encode(delta)
	})

	mux.HandleFunc("GET /api/coverage/lcov", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w)
		w.Header().Set("Content-Type", "text/plain")
		name := r.URL.Query().Get("name")
		snap := findSnap(store, name)
		if snap == nil {
			http.Error(w, "snapshot not found", http.StatusNotFound)
			return
		}
		fmt.Fprint(w, SnapshotToLcov(snap))
	})

	mux.HandleFunc("OPTIONS /api/", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w)
		w.WriteHeader(http.StatusNoContent)
	})

	addr := "127.0.0.1:" + strconv.Itoa(port)
	log.Printf("coverage API listening on http://%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("coverage API server: %v", err)
	}
}

func setCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")
}

func findSnap(store Store, name string) *Snapshot {
	if store == nil {
		return nil
	}
	for _, s := range store.Snapshots() {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// FormatSnapshotLcov formats a snapshot as a simple lcov tracefile.
func FormatSnapshotLcov(snap *Snapshot) string {
	var b strings.Builder
	for url, cov := range snap.Scripts {
		fmt.Fprintf(&b, "SF:%s\n", url)
		for line, count := range cov.Lines {
			fmt.Fprintf(&b, "DA:%d,%d\n", line, count)
		}
		b.WriteString("end_of_record\n")
	}
	return b.String()
}
