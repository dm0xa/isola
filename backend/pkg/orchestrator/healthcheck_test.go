package orchestrator

import "testing"

// TestRuntimeUnreachable pins the discriminator used by the health checker to
// tell "the runtime could not be reached" (skip this round) from "the runtime
// answered, the container is not there" (count a miss towards finalization).
//
// Both cases exit non-zero, so the exit code cannot be used, and the strings
// below are the real ones observed from the two CLIs:
//   - `podman inspect` on a removed container: `Error: no such object: "..."`,
//     exit 125.
//   - `docker -H <podman socket> inspect` on a removed container: the compat
//     API tries to read the bare hex ID as an image name and fails with an
//     image-name normalization error, exit 1. Missing this string is what let a
//     removed container look like an unreachable daemon and leak the session.
func TestRuntimeUnreachable(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
		{
			name:   "podman: removed container",
			output: "Error: no such object: \"033214aab4ab171c54a82427eb6ad346ff78b47655c9831e4a9e008267737b09\"\n",
			want:   false,
		},
		{
			name:   "docker cli over podman compat api: removed container",
			output: `Error response from daemon: normalizing image: normalizing name "033214aab4ab171c54a82427eb6ad346ff78b47655c9831e4a9e008267737b09" (orig: "033214aab4ab171c54a82427eb6ad346ff78b47655c9831e4a9e008267737b09") for compat API: invalid repository name (033214aab4ab171c54a82427eb6ad346ff78b47655c9831e4a9e008267737b09), cannot specify 64-byte hexadecimal strings`,
			want:   false,
		},
		{
			name:   "missing socket",
			output: "failed to connect to the docker API at unix:///tmp/definitely-no-socket.sock; check if the path is correct and if the daemon is running: dial unix /tmp/definitely-no-socket.sock: connect: no such file or directory",
			want:   true,
		},
		{
			name:   "daemon down",
			output: "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
			want:   true,
		},
		{
			name:   "connection refused",
			output: "Error: failed to connect: dial tcp 127.0.0.1:2375: connect: connection refused",
			want:   true,
		},
		{
			name:   "probe timed out",
			output: "error during connect: context deadline exceeded",
			want:   true,
		},
		{
			name:   "case insensitive",
			output: "FAILED TO CONNECT TO THE DOCKER API AT UNIX:///tmp/x.sock",
			want:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := runtimeUnreachable(tc.output); got != tc.want {
				t.Errorf("runtimeUnreachable(%q) = %v, want %v", tc.output, got, tc.want)
			}
		})
	}
}
