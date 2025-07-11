package routes

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// HandleUpload handles POST /upload for authenticated image uploads
func HandleUpload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-Token") != os.Getenv("UPLOAD_TOKEN") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10MB max
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Invalid file", http.StatusBadRequest)
			return
		}
		defer file.Close()

		buf := make([]byte, 512)
		if _, err := file.Read(buf); err != nil {
			http.Error(w, "Could not read file", http.StatusBadRequest)
			return
		}
		filetype := http.DetectContentType(buf)
		allowedTypes := map[string]bool{"image/jpeg": true, "image/png": true, "image/gif": true}
		if !allowedTypes[filetype] {
			http.Error(w, "Unsupported file type", http.StatusBadRequest)
			return
		}
		file.Seek(0, io.SeekStart)

		filename := filepath.Base(header.Filename)
		out, err := os.Create(filepath.Join("upload", filename))
		if err != nil {
			http.Error(w, "Unable to save file", http.StatusInternalServerError)
			return
		}
		defer out.Close()

		if _, err := io.Copy(out, file); err != nil {
			http.Error(w, "Failed to save file", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("Upload successful"))
	}
}

// HandleUploads serves files from the ./upload directory at /uploads/*
func HandleUploads() http.Handler {
	return http.StripPrefix("/uploads/", http.FileServer(http.Dir("./upload")))
}
