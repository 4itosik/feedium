package postgres_test

import (
	"testing"
)

// All tests in this file are skipped because they require integration testing
// with a real database. go-mocket has limitations with:
// - Complex JOIN queries with UUID scanning
// - Array aggregation functions (array_agg)
// - Multiple query result sets in sequence
//
// These tests should be run against a real PostgreSQL database in integration tests.

func TestSummaryRepository_GetByPostID_Found(t *testing.T) {
	t.Skip("requires integration test with real database")
}

func TestSummaryRepository_GetByPostID_NotFound(t *testing.T) {
	t.Skip("requires integration test with real database")
}

func TestSummaryRepository_ListSummaries_AllSources(t *testing.T) {
	t.Skip("requires integration test with real database")
}

func TestSummaryRepository_ListSummaries_WithSourceFilter(t *testing.T) {
	t.Skip("requires integration test with real database")
}

func TestSummaryRepository_ListSummaries_EmptyResult(t *testing.T) {
	t.Skip("requires integration test with real database")
}
