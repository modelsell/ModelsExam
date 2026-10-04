package claudecheck

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTransientBaselineUsesPlannedStreamOnce(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response Response
		err      error
	}{
		{"timeout", Response{}, context.DeadlineExceeded},
		{"overload", Response{Status: 503}, errors.New("system cpu overloaded")},
		{"malformed", Response{Status: 200, Body: []byte(`{}`)}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := happyTransport(t, true)
			calls, streams := 0, 0
			r := Run(context.Background(), Options{Model: "claude"}, func(ctx context.Context, req Request) (Response, error) {
				calls++
				deadline, ok := ctx.Deadline()
				require.True(t, ok)
				require.Greater(t, time.Until(deadline), 35*time.Second)
				require.LessOrEqual(t, time.Until(deadline), ProbeTimeout)
				if !req.Count && req.Body["stream"] == true {
					streams++
					if calls == 2 {
						require.Equal(t, "Respond with exactly PONG and no other text.", req.Body["system"])
					}
				}
				if calls == 1 {
					return tc.response, tc.err
				}
				return call(ctx, req)
			})
			require.Equal(t, 8, calls)
			require.Equal(t, 1, streams)
			require.Equal(t, "stream", r.BaselineProbe)
			require.Empty(t, r.StopReason)
			require.False(t, r.Cancelled)
			for _, id := range []string{"stream", "tool", "system", "multi_turn", "zero_output", "token_count"} {
				require.Equal(t, "pass", resultCheck(t, r, id).Status, id)
			}
			if tc.name != "malformed" {
				require.Equal(t, "inconclusive", resultCheck(t, r, "basic").Status)
				require.Empty(t, r.Samples[0].ValidationErrors)
			}
			require.Equal(t, "stream", resultCheck(t, r, "model_echo").Evidence["baseline_probe"])
			require.Equal(t, 90, r.Limits.ProbeTimeoutSeconds)
			require.Equal(t, 600, r.Limits.RunTimeoutSeconds)
		})
	}
}

func TestUnusableBaselineStopsWithSpecificCause(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response Response
		err      error
		code     string
		requests int
	}{
		{"credentials", Response{Status: 401}, errors.New("invalid key"), "access_denied", 1},
		{"group unavailable", Response{Status: 503}, errors.New("No available channel for model claude-opus-5 under group VT-2"), "route_unavailable", 1},
		{"both time out", Response{}, context.DeadlineExceeded, "probe_timeout", 2},
		{"both unavailable", Response{Status: 503}, errors.New("system cpu overloaded"), "upstream_unavailable", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			r := Run(context.Background(), Options{Model: "claude", PDF: true, StreamComparison: true, Thinking: true}, func(context.Context, Request) (Response, error) { calls++; return tc.response, tc.err })
			require.Equal(t, tc.requests, calls)
			require.Equal(t, tc.code, r.StopReason)
			require.Equal(t, r.Samples[len(r.Samples)-1].Probe, r.StopProbe)
			require.Equal(t, "inconclusive", resultCheck(t, r, "basic").Status)
			require.Equal(t, "run_stopped", resultCheck(t, r, "tool").Code)
			require.False(t, r.Cancelled)
			require.Zero(t, r.Summary["fail"])
		})
	}
}

func TestCallerDeadlineAndCancellationRemainAuthoritative(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if timeout {
			cancel()
			ctx, cancel = context.WithTimeout(context.Background(), 5*time.Millisecond)
		}
		calls := 0
		r := Run(ctx, Options{Model: "claude"}, func(ctx context.Context, _ Request) (Response, error) {
			calls++
			if !timeout {
				cancel()
			}
			<-ctx.Done()
			return Response{}, ctx.Err()
		})
		cancel()
		require.Equal(t, 1, calls)
		require.True(t, r.Cancelled)
		if timeout {
			require.Equal(t, "run_timeout", r.StopReason)
		} else {
			require.Equal(t, "cancelled", r.StopReason)
		}
	}
}

func TestLaterTransportTimeoutDoesNotScoreAsModelMismatch(t *testing.T) {
	call := happyTransport(t, true)
	calls := 0
	r := Run(context.Background(), Options{Model: "claude"}, func(ctx context.Context, req Request) (Response, error) {
		calls++
		if calls == 4 { // Tool request after basic, CountTokens and stream.
			return Response{}, context.DeadlineExceeded
		}
		return call(ctx, req)
	})
	require.Equal(t, 8, calls)
	require.Equal(t, "inconclusive", resultCheck(t, r, "tool").Status)
	require.Equal(t, "probe_timeout", resultCheck(t, r, "tool").Code)
	require.Equal(t, "pass", resultCheck(t, r, "multi_turn").Status)
	require.Empty(t, r.StopReason)
}
