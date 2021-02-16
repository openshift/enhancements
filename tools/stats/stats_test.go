package stats

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddWithCascade(t *testing.T) {
	first := Bucket{
		Rule: func(details *PullRequestDetails) bool {
			return true
		},
		Cascade: true,
	}
	second := Bucket{
		Rule: func(details *PullRequestDetails) bool {
			return true
		},
	}

	s := Stats{
		Buckets: []*Bucket{
			&first,
			&second,
		},
	}
	details := &PullRequestDetails{}
	s.add(details)
	assert.Equal(t, 1, len(first.Requests))
	assert.Equal(t, 1, len(second.Requests))
}

func TestAddWithoutCascade(t *testing.T) {
	first := Bucket{
		Rule: func(details *PullRequestDetails) bool {
			return true
		},
	}
	second := Bucket{
		Rule: func(details *PullRequestDetails) bool {
			return true
		},
	}

	s := Stats{
		Buckets: []*Bucket{
			&first,
			&second,
		},
	}
	details := &PullRequestDetails{}
	s.add(details)
	assert.Equal(t, 1, len(first.Requests))
	assert.Equal(t, 0, len(second.Requests))
}

func TestAddWithoutMatch(t *testing.T) {
	first := Bucket{
		Rule: func(details *PullRequestDetails) bool {
			return false
		},
	}
	second := Bucket{
		Rule: func(details *PullRequestDetails) bool {
			return false
		},
	}

	s := Stats{
		Buckets: []*Bucket{
			&first,
			&second,
		},
	}
	details := &PullRequestDetails{}
	s.add(details)
	assert.Equal(t, 0, len(first.Requests))
	assert.Equal(t, 0, len(second.Requests))
}
