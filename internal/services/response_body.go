package services

import (
	"fmt"
	"io"
)

const (
	maxProviderResponseBodyBytes int64 = 32 << 20
	maxProviderErrorBodyBytes    int64 = 1 << 20
)

func readBoundedResponseBody(body io.Reader, limit int64) ([]byte, error) {
	reader := &io.LimitedReader{R: body, N: limit + 1}
	data, err := io.ReadAll(reader)
	if err != nil {
		return data, err
	}
	if int64(len(data)) > limit {
		return data[:limit], fmt.Errorf("provider response body exceeds %d bytes", limit)
	}
	return data, nil
}

func readProviderErrorBody(body io.Reader) []byte {
	data, _ := readBoundedResponseBody(body, maxProviderErrorBodyBytes)
	return data
}
