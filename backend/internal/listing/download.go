package listing

import (
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"path"
)

type DownloadRequest struct {
	Path           string
	RangeRequested bool
}

func (s *Service) ServeDownload(w http.ResponseWriter, r *http.Request, relativePath string) (DownloadRequest, int64, int64, error) {
	file, info, err := s.OpenFile(relativePath)
	if err != nil {
		return DownloadRequest{}, 0, 0, err
	}
	defer file.Close()

	cleanPath, err := normalize(relativePath)
	if err != nil {
		return DownloadRequest{}, 0, 0, err
	}
	filename := path.Base(cleanPath)
	if value := mime.FormatMediaType("attachment", map[string]string{"filename": filename}); value != "" {
		w.Header().Set("Content-Disposition", value)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "application/octet-stream")

	counted := &countingWriter{ResponseWriter: w}
	http.ServeContent(counted, r, filename, info.ModTime(), file)

	return DownloadRequest{
		Path:           cleanPath,
		RangeRequested: r.Header.Get("Range") != "",
	}, counted.bytesWritten, info.Size(), nil
}

type countingWriter struct {
	http.ResponseWriter
	bytesWritten int64
}

func (w *countingWriter) Write(value []byte) (int, error) {
	written, err := w.ResponseWriter.Write(value)
	w.bytesWritten += int64(written)
	return written, err
}

func DownloadErrorStatus(err error) int {
	switch {
	case errors.Is(err, ErrInvalidPath):
		return http.StatusBadRequest
	case errors.Is(err, ErrNotFound), errors.Is(err, fs.ErrNotExist):
		return http.StatusNotFound
	case errors.Is(err, ErrNotFile):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
