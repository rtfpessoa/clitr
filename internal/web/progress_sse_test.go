package web

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSEStreamSerializesConcurrentWrites(t *testing.T) {
	recorder := httptest.NewRecorder()
	stream := newSSEStream(recorder, recorder)
	var writers sync.WaitGroup
	for i := range 32 {
		writers.Add(1)
		go func() {
			defer writers.Done()
			stream.write("event: progress\ndata: %d\n\n", i)
		}()
	}
	writers.Wait()

	body := recorder.Body.String()
	require.Equal(t, 32, strings.Count(body, "event: progress\n"))
	for i := range 32 {
		require.Contains(t, body, fmt.Sprintf("data: %d\n\n", i))
	}
}
