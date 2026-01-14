.PHONY: build-test clean

# Build the app locally using GoReleaser without publishing
build-test:
	goreleaser release --snapshot --clean

# Clean up the dist folder
clean:
	rm -rf dist/
