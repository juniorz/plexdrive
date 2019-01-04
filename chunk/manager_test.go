package chunk

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	. "github.com/claudetech/loggo/default"
	"github.com/dweidenfeld/plexdrive/config"
	"github.com/dweidenfeld/plexdrive/drive"
)

const (
	chunkSize = 4096
	lookAhead = 2
	maxChunks = 5
)

func init() {
	Log.SetFormat("[{{.File}}:{{.Line}}] [{{.TimeStr}}] {{.LevelStr}}: {{.Content}}")
}

func Test_Manager_DownloadsChunkAndLookAheads(t *testing.T) {
	rangeRegex := regexp.MustCompile("bytes=([0-9]+)-([0-9]+)")
	handlerFn := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() != "/test/file.txt" {
			t.Errorf("Unexpected request URL, %v is found.", r.URL)
		}

		headerRange := r.Header.Get("Range")
		match := rangeRegex.FindStringSubmatch(headerRange)
		if len(match) != 3 {
			t.Errorf("Unexpected range header, %v is found.", headerRange)
		}

		start, _ := strconv.Atoi(match[1])
		end, _ := strconv.Atoi(match[2])
		requestedLen := end - start + 1

		if requestedLen != chunkSize {
			t.Errorf("Unexpected range, %v is found.", requestedLen)
		}

		chunkIdx := byte((start % chunkSize) % 256)

		w.Header().Set("Content-Length", fmt.Sprintf("%d", requestedLen))
		w.WriteHeader(http.StatusPartialContent)

		ret := make([]byte, requestedLen)
		for i := 0; i < requestedLen; i++ {
			ret[i] = chunkIdx
		}

		w.Write(ret)
	})

	ts := httptest.NewServer(handlerFn)
	defer ts.Close()

	cfg := &config.Config{}

	dir, err := ioutil.TempDir("", "plexdrive-test")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir) // clean up
	cache, err := drive.NewCache(filepath.Join(dir, "cache.bolt"), dir, true)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	tokenJSON := []byte(`{"access_token":"ACCESS_TOKEN","token_type":"Bearer","refresh_token":"NEW_REFRESH_TOKEN","expiry":"3000-12-30T23:47:26.368654116Z"}`)
	tokenFile := filepath.Join(dir, "token.json")
	if err := ioutil.WriteFile(tokenFile, tokenJSON, 0666); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	refresh, _ := time.ParseDuration("100000h")
	client, err := drive.NewClient(cfg, cache, refresh, "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	object := &drive.APIObject{
		ObjectID:     "test/file.txt",
		Name:         "file.txt",
		Size:         uint64(chunkSize * 10),
		LastModified: time.Now(),
		DownloadURL:  fmt.Sprintf("%s/test/file.txt", ts.URL),
		Parents:      []string{"test/"},
	}

	m, err := NewManager(int64(chunkSize), 0, 1, 1, client, maxChunks)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer m.Close()

	go m.thread()

	// Request twice the chunkSize
	response := make(chan Response)
	m.GetChunk(object, 0, int64(chunkSize*2), response)
	res := <-response
	if res.Error != nil {
		t.Errorf("Unexpected error: %v", res.Error)
	}

	// FIXME: It always returns at maximum one chunk
	if len(res.Bytes) != chunkSize { // expected to return everything requested
		t.Errorf("Unexpected response size: %v", len(res.Bytes))
	}

	// Request the next chunk
	response = make(chan Response)
	m.GetChunk(object, chunkSize*2+100, int64(chunkSize), response)

	res = <-response
	if res.Error != nil {
		t.Errorf("Unexpected error: %v", res.Error)
	}

	// FIXME: Returns less than requested due reaching chunk boundary
	if len(res.Bytes) != chunkSize-100 {
		t.Errorf("Unexpected response size: %v", len(res.Bytes))
	}

	//TODO: What about?
	//m.GetChunk(object, 0, 4096/2, response)
}
