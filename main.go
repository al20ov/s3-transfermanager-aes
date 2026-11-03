package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// "github.com/goforj/godump"

var bucket string

var secretHex string = "6368616e676520746869732070617373" // spells out "change this pass" so CHANGE THIS

const maxBodySize int64 = 32 << 28 // 8GiB

type AppState struct {
	transfermanagerClient *transfermanager.Client
	s3Client              *s3.Client
	ctx                   *context.Context
}

func main() {
	ctx := context.Background()

	bucket = os.Getenv("BUCKET_NAME")
	if bucket == "" {
		panic("BUCKET_NAME is required")
	}

	sdkConfig, err := config.LoadDefaultConfig(ctx)

	server := http.NewServeMux()

	if err != nil {
		panic(err)
	}

	s3Client := s3.NewFromConfig(sdkConfig)
	transfermanagerClient := transfermanager.New(s3Client)

	appState := AppState{
		transfermanagerClient: transfermanagerClient,
		s3Client:              s3Client,
		ctx:                   &ctx,
	}

	server.HandleFunc("POST /{path...}", appState.PutObjectHandler)
	server.HandleFunc("PUT /{path...}", appState.PutObjectHandler)

	server.HandleFunc("GET /{path...}", appState.GetObjectHandler)

	server.HandleFunc("DELETE /{path...}", appState.DeleteObjectHandler)

	http.ListenAndServe("0.0.0.0:8080", server)
}

func (s *AppState) PutObjectHandler(w http.ResponseWriter, request *http.Request) {
	path := request.PathValue("path")

	request.Body = http.MaxBytesReader(w, request.Body, maxBodySize)

	file := request.Body

	defer file.Close()

	// key comes from the last segment of the path
	key := strings.TrimPrefix(path, "/")
	fullPath := path

	contentType := mime.TypeByExtension(filepath.Ext(fullPath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	// aes stuff

	secret, err := hex.DecodeString(secretHex)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	block, err := aes.NewCipher(secret)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	iv := make([]byte, aes.BlockSize)

	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stream := cipher.NewCTR(block, iv)

	reader := &cipher.StreamReader{S: stream, R: file}

	// END aes stuff
	_, err = s.transfermanagerClient.UploadObject(*s.ctx, &transfermanager.UploadObjectInput{
		Bucket:      &bucket,
		Key:         &fullPath,
		ContentType: &contentType,
		Body:        reader,
		Metadata: map[string]string{
			"encryption": "aes-128-ctr",
			"iv":         hex.EncodeToString(iv),
		},
	})

	if err != nil {
		var maxBytesErr *http.MaxBytesError

		if errors.As(err, &maxBytesErr) {
			http.Error(w, fmt.Sprintf("request body over %d byte limit", maxBytesErr.Limit), http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Println("Sent file", key, "to", bucket)
	fmt.Fprintln(w, "OK")
}

func (s *AppState) GetObjectHandler(w http.ResponseWriter, request *http.Request) {
	path := request.PathValue("path")

	if strings.HasSuffix(path, "/") {
		http.Error(w, "Object not found", http.StatusNotFound)
		return
	}
	filename := strings.TrimPrefix(path, "/")

	output, err := s.transfermanagerClient.GetObject(*s.ctx, &transfermanager.GetObjectInput{
		Bucket: &bucket,
		Key:    &path,
	})

	if err != nil {
		http.Error(w, "Error retrieving file", http.StatusInternalServerError)
		return
	}

	if output.Body == nil {
		http.Error(w, "Object not found", http.StatusNotFound)
		return
	}

	// aes decrypt
	secret, err := hex.DecodeString(secretHex)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	block, err := aes.NewCipher(secret)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ivHex, ok := output.Metadata["iv"]
	if !ok {
		http.Error(w, "Object has no encryption metadata", http.StatusBadRequest)
		return
	}
	iv, err := hex.DecodeString(ivHex)
	if err != nil || len(iv) != aes.BlockSize {
		http.Error(w, "Invalid encryption metadata", http.StatusBadRequest)
		return
	}

	stream := cipher.NewCTR(block, iv)
	reader := &cipher.StreamReader{S: stream, R: output.Body}

	// END aes decrypt
	contentType := "application/octet-stream"
	if output.ContentType != nil {
		contentType = *output.ContentType
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filename))

	_, err = io.Copy(w, reader)
	if err != nil {
		http.Error(w, "Error reading file", http.StatusInternalServerError)
		return
	}
}

func (s *AppState) DeleteObjectHandler(w http.ResponseWriter, request *http.Request) {
	path := request.PathValue("path")

	if strings.HasSuffix(path, "/") {
		http.Error(w, "Object not found", http.StatusNotFound)
		return
	}

	_, err := s.s3Client.DeleteObject(*s.ctx, &s3.DeleteObjectInput{
		Bucket: &bucket,
		Key:    &path,
	})

	if err != nil {
		var notFoundErr *types.NoSuchKey

		if errors.As(err, &notFoundErr) {
			http.Error(w, "Object not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Error deleting object", http.StatusInternalServerError)
		return
	}
	fmt.Println("Deleted", path, "from", bucket)
	fmt.Fprintln(w, "OK")
}
