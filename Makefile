# Makefile
.PHONY: lint test test-unit test-integration build clean

# Menjalankan linter
lint:
	golangci-lint run ./...

# Menjalankan unit test (mengabaikan file dengan tag integration)
test-unit:
	go test -short -v ./...

# Menjalankan integration test secara spesifik
test-integration:
	go test -tags=integration -v ./...

# Menjalankan seluruh test
test: test-unit test-integration

# Membersihkan file binari
clean:
	rm -rf bin/
