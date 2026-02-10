.PHONY: run build build-static clean

# Default: Run locally
run:
	go run .

# Build for local machine
build:
	go build -o slurm-dashboard-local .

# Build static binary for HPC
build-static:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o slurm-dashboard .

clean:
	rm -f slurm-dashboard slurm-dashboard-local slurm-dashboard-static
