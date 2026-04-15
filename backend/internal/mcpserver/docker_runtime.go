package mcpserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type DockerRuntimeConfig struct {
	Enabled               bool
	Host                  string
	SessionTTLSeconds     int
	DefaultTimeoutSeconds int
	MaxTimeoutSeconds     int
	MemoryLimit           string
	CPULimit              float64
	PidsLimit             int
	WorkspaceRoot         string
}

type dockerSession struct {
	ID            string
	Kind          string
	Image         string
	ContainerName string
	WorkspaceDir  string
	CreatedAt     time.Time
	LastUsedAt    time.Time
	mu            sync.Mutex
}

type dockerExecResult struct {
	Stdout     string
	Stderr     string
	ExitCode   int
	DurationMs int64
}

type dockerRuntime struct {
	cfg       DockerRuntimeConfig
	dockerBin string
	mu        sync.RWMutex
	sessions  map[string]*dockerSession
	stopCh    chan struct{}
	stopped   chan struct{}
}

var dockerRT *dockerRuntime

func InitDockerRuntime(cfg DockerRuntimeConfig) error {
	if dockerRT != nil {
		dockerRT.Close()
		dockerRT = nil
	}
	if !cfg.Enabled {
		log.Printf("[docker-runtime] disabled")
		return nil
	}
	if err := os.MkdirAll(cfg.WorkspaceRoot, 0o755); err != nil {
		return fmt.Errorf("create docker workspace root: %w", err)
	}
	dockerBin, err := exec.LookPath("docker")
	if err != nil {
		return fmt.Errorf("docker binary not found: %w", err)
	}
	rt := &dockerRuntime{
		cfg:       cfg,
		dockerBin: dockerBin,
		sessions:  make(map[string]*dockerSession),
		stopCh:    make(chan struct{}),
		stopped:   make(chan struct{}),
	}
	dockerRT = rt
	go rt.cleanupLoop()
	log.Printf("[docker-runtime] enabled workspace_root=%s ttl=%ds", cfg.WorkspaceRoot, cfg.SessionTTLSeconds)
	return nil
}

func IsDockerRuntimeEnabled() bool {
	return dockerRT != nil
}

// DockerRuntimeConfigFromExternal 供外部包构建 mcpserver 运行时配置.
func DockerRuntimeConfigFromExternal(
	enabled bool,
	host string,
	sessionTTLSeconds int,
	defaultTimeoutSeconds int,
	maxTimeoutSeconds int,
	memoryLimit string,
	cpuLimit float64,
	pidsLimit int,
	workspaceRoot string,
) DockerRuntimeConfig {
	return DockerRuntimeConfig{
		Enabled:               enabled,
		Host:                  host,
		SessionTTLSeconds:     sessionTTLSeconds,
		DefaultTimeoutSeconds: defaultTimeoutSeconds,
		MaxTimeoutSeconds:     maxTimeoutSeconds,
		MemoryLimit:           memoryLimit,
		CPULimit:              cpuLimit,
		PidsLimit:             pidsLimit,
		WorkspaceRoot:         workspaceRoot,
	}
}

func (rt *dockerRuntime) Close() {
	close(rt.stopCh)
	<-rt.stopped

	rt.mu.Lock()
	sessions := make([]*dockerSession, 0, len(rt.sessions))
	for _, s := range rt.sessions {
		sessions = append(sessions, s)
	}
	rt.sessions = map[string]*dockerSession{}
	rt.mu.Unlock()

	for _, s := range sessions {
		_ = rt.removeContainer(s.ContainerName)
		_ = os.RemoveAll(s.WorkspaceDir)
	}
}

func (rt *dockerRuntime) cleanupLoop() {
	defer close(rt.stopped)
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-rt.stopCh:
			return
		case <-ticker.C:
			rt.cleanupExpiredSessions()
		}
	}
}

func (rt *dockerRuntime) cleanupExpiredSessions() {
	now := time.Now()
	ttl := time.Duration(rt.cfg.SessionTTLSeconds) * time.Second
	var expired []string

	rt.mu.RLock()
	for id, s := range rt.sessions {
		if now.Sub(s.LastUsedAt) > ttl {
			expired = append(expired, id)
		}
	}
	rt.mu.RUnlock()

	for _, id := range expired {
		if _, err := rt.closeSession(id); err != nil {
			log.Printf("[docker-runtime] cleanup session=%s err=%v", id, err)
		}
	}
}

func (rt *dockerRuntime) ensureSession(kind string, sessionID string, image string) (*dockerSession, bool, error) {
	now := time.Now()
	if strings.TrimSpace(sessionID) == "" {
		sessionID = fmt.Sprintf("%s-%d", kind, now.UnixNano())
	}
	sessionID = sanitizeSessionID(sessionID)
	if sessionID == "" {
		return nil, false, fmt.Errorf("invalid session_id")
	}

	rt.mu.Lock()
	if s, ok := rt.sessions[sessionID]; ok {
		s.LastUsedAt = now
		rt.mu.Unlock()
		if s.Kind != kind {
			return nil, false, fmt.Errorf("session kind mismatch: expected %s got %s", kind, s.Kind)
		}
		if s.Image != image {
			return nil, false, fmt.Errorf("session image mismatch")
		}
		return s, false, nil
	}

	containerName := fmt.Sprintf("mcp-%s-%s", kind, sessionID)
	workspaceDir := filepath.Join(rt.cfg.WorkspaceRoot, kind, sessionID)
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		rt.mu.Unlock()
		return nil, false, fmt.Errorf("create workspace: %w", err)
	}

	s := &dockerSession{
		ID:            sessionID,
		Kind:          kind,
		Image:         image,
		ContainerName: containerName,
		WorkspaceDir:  workspaceDir,
		CreatedAt:     now,
		LastUsedAt:    now,
	}
	rt.sessions[sessionID] = s
	rt.mu.Unlock()

	if err := rt.ensureImage(image); err != nil {
		rt.mu.Lock()
		delete(rt.sessions, sessionID)
		rt.mu.Unlock()
		return nil, false, err
	}
	if err := rt.startSessionContainer(s); err != nil {
		rt.mu.Lock()
		delete(rt.sessions, sessionID)
		rt.mu.Unlock()
		return nil, false, err
	}
	return s, true, nil
}

func (rt *dockerRuntime) getSession(sessionID string) (*dockerSession, error) {
	rt.mu.RLock()
	s, ok := rt.sessions[sessionID]
	rt.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	return s, nil
}

func (rt *dockerRuntime) closeSession(sessionID string) (bool, error) {
	rt.mu.Lock()
	s, ok := rt.sessions[sessionID]
	if ok {
		delete(rt.sessions, sessionID)
	}
	rt.mu.Unlock()
	if !ok {
		return false, nil
	}
	if err := rt.removeContainer(s.ContainerName); err != nil {
		return true, err
	}
	if err := os.RemoveAll(s.WorkspaceDir); err != nil {
		return true, fmt.Errorf("remove workspace: %w", err)
	}
	return true, nil
}

func (rt *dockerRuntime) startSessionContainer(s *dockerSession) error {
	baseArgs := []string{"run", "-d", "--rm", "--name", s.ContainerName,
		"--network=none",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--workdir", "/workspace",
		"--cap-drop=ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", strconv.Itoa(rt.cfg.PidsLimit),
		"--memory", rt.cfg.MemoryLimit,
		"--cpus", fmt.Sprintf("%.2f", rt.cfg.CPULimit),
		"-v", s.WorkspaceDir + ":/workspace:rw",
	}
	finalTail := []string{s.Image, "tail", "-f", "/dev/null"}

	args := append([]string{}, baseArgs...)
	usedUploadMount := false
	if uploadMount, err := buildFrontendUploadMountArg(); err == nil {
		args = append(args, "-v", uploadMount)
		usedUploadMount = true
	} else {
		log.Printf("[docker-runtime] skip upload mount: %v", err)
	}
	args = append(args, finalTail...)
	if _, _, err := rt.runDockerCmd(context.Background(), 60*time.Second, args...); err != nil {
		if usedUploadMount {
			log.Printf("[docker-runtime] start with upload mount failed, retry without upload mount: %v", err)
			fallbackArgs := append(append([]string{}, baseArgs...), finalTail...)
			if _, _, retryErr := rt.runDockerCmd(context.Background(), 60*time.Second, fallbackArgs...); retryErr == nil {
				return nil
			}
		}
		return fmt.Errorf("start container: %w", err)
	}
	return nil
}

func buildFrontendUploadMountArg() (string, error) {
	uploadRoot, err := resolveFrontendUploadRootDir()
	if err != nil {
		return "", err
	}
	absUploadRoot, err := filepath.Abs(uploadRoot)
	if err != nil {
		return "", fmt.Errorf("abs upload root: %w", err)
	}
	// Mount to the same absolute path so paths injected into prompts remain directly usable.
	return absUploadRoot + ":" + absUploadRoot + ":ro", nil
}

func (rt *dockerRuntime) ensureImage(image string) error {
	if strings.TrimSpace(image) == "mcp-code-runner:latest" {
		return rt.ensureCodeRunnerImage()
	}
	if _, _, err := rt.runDockerCmd(context.Background(), 30*time.Second, "image", "inspect", image); err == nil {
		return nil
	}
	_, _, err := rt.runDockerCmd(context.Background(), 300*time.Second, "pull", image)
	if err != nil {
		return fmt.Errorf("pull image %s: %w", image, err)
	}
	return nil
}

func (rt *dockerRuntime) ensureCodeRunnerImage() error {
	name := "mcp-code-runner:latest"
	if _, _, err := rt.runDockerCmd(context.Background(), 30*time.Second, "image", "inspect", name); err == nil {
		return nil
	}
	dockerfile, contextDir, err := resolveCodeRunnerDockerfile()
	if err != nil {
		return err
	}
	_, _, err = rt.runDockerCmdWithDir(context.Background(), 600*time.Second, contextDir, "build", "-t", name, "-f", dockerfile, contextDir)
	if err != nil {
		return fmt.Errorf("build code runner image: %w", err)
	}
	return nil
}

func resolveCodeRunnerDockerfile() (dockerfile string, contextDir string, err error) {
	if _, thisFile, _, ok := runtime.Caller(0); ok {
		baseDir := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
		abs := filepath.Join(baseDir, "docker", "code-runner", "Dockerfile")
		if _, err := os.Stat(abs); err == nil {
			return abs, filepath.Dir(abs), nil
		}
	}
	candidates := []string{
		filepath.Join("docker", "code-runner", "Dockerfile"),
		filepath.Join("backend", "docker", "code-runner", "Dockerfile"),
	}
	for _, c := range candidates {
		abs, e := filepath.Abs(c)
		if e != nil {
			continue
		}
		if _, e := os.Stat(abs); e == nil {
			return abs, filepath.Dir(abs), nil
		}
	}
	return "", "", fmt.Errorf("code runner Dockerfile not found")
}

func (rt *dockerRuntime) removeContainer(containerName string) error {
	_, _, err := rt.runDockerCmd(context.Background(), 30*time.Second, "rm", "-f", containerName)
	if err != nil && !strings.Contains(err.Error(), "No such container") {
		return err
	}
	return nil
}

func (rt *dockerRuntime) execInSession(s *dockerSession, timeoutSeconds int, stdin string, cmdArgs ...string) (dockerExecResult, error) {
	timeout := clampTimeout(timeoutSeconds, rt.cfg.DefaultTimeoutSeconds, rt.cfg.MaxTimeoutSeconds)
	start := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastUsedAt = time.Now()

	args := []string{"exec", "-i", s.ContainerName}
	args = append(args, cmdArgs...)
	stdout, stderr, err := rt.runDockerCmdWithInput(context.Background(), time.Duration(timeout)*time.Second, stdin, args...)
	res := dockerExecResult{
		Stdout:     stdout,
		Stderr:     stderr,
		DurationMs: time.Since(start).Milliseconds(),
	}
	if err != nil {
		res.ExitCode = extractExitCode(err)
		return res, err
	}
	res.ExitCode = 0
	return res, nil
}

func (rt *dockerRuntime) installPythonPackages(s *dockerSession, packages []string, timeoutSeconds int) (dockerExecResult, error) {
	timeout := clampTimeout(timeoutSeconds, rt.cfg.DefaultTimeoutSeconds, rt.cfg.MaxTimeoutSeconds)
	if len(packages) == 0 {
		return dockerExecResult{ExitCode: 0}, nil
	}
	cleanPkgs := make([]string, 0, len(packages))
	for _, p := range packages {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		cleanPkgs = append(cleanPkgs, p)
	}
	if len(cleanPkgs) == 0 {
		return dockerExecResult{ExitCode: 0}, nil
	}
	start := time.Now()
	args := []string{
		"run", "--rm",
		"--workdir", "/workspace",
		"--memory", rt.cfg.MemoryLimit,
		"--cpus", fmt.Sprintf("%.2f", rt.cfg.CPULimit),
		"--pids-limit", strconv.Itoa(rt.cfg.PidsLimit),
		"--cap-drop=ALL",
		"--security-opt", "no-new-privileges",
		"-v", s.WorkspaceDir + ":/workspace:rw",
		s.Image,
		"python", "-m", "pip", "install", "--no-input", "-t", "/workspace/.deps",
	}
	args = append(args, cleanPkgs...)
	stdout, stderr, err := rt.runDockerCmd(context.Background(), time.Duration(timeout)*time.Second, args...)
	res := dockerExecResult{Stdout: stdout, Stderr: stderr, DurationMs: time.Since(start).Milliseconds()}
	if err != nil {
		res.ExitCode = extractExitCode(err)
		return res, err
	}
	res.ExitCode = 0
	return res, nil
}

func (rt *dockerRuntime) listArtifacts(s *dockerSession) ([]string, error) {
	var files []string
	err := filepath.WalkDir(s.WorkspaceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.WorkspaceDir, path)
		if err != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	if len(files) > 200 {
		files = files[:200]
	}
	return files, nil
}

func (rt *dockerRuntime) runDockerCmd(ctx context.Context, timeout time.Duration, args ...string) (string, string, error) {
	return rt.runDockerCmdWithDir(ctx, timeout, "", args...)
}

func (rt *dockerRuntime) runDockerCmdWithDir(ctx context.Context, timeout time.Duration, dir string, args ...string) (string, string, error) {
	return rt.runDockerCmdWithInputAndDir(ctx, timeout, "", dir, args...)
}

func (rt *dockerRuntime) runDockerCmdWithInput(ctx context.Context, timeout time.Duration, stdin string, args ...string) (string, string, error) {
	return rt.runDockerCmdWithInputAndDir(ctx, timeout, stdin, "", args...)
}

func (rt *dockerRuntime) runDockerCmdWithInputAndDir(ctx context.Context, timeout time.Duration, stdin string, dir string, args ...string) (string, string, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, rt.dockerBin, args...)
	if strings.TrimSpace(rt.cfg.Host) != "" {
		cmd.Env = append(os.Environ(), "DOCKER_HOST="+rt.cfg.Host)
	}
	if dir != "" {
		cmd.Dir = dir
	}
	var outb bytes.Buffer
	var errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	err := cmd.Run()
	stdout := outb.String()
	stderr := errb.String()
	if cctx.Err() == context.DeadlineExceeded {
		if strings.TrimSpace(stderr) == "" {
			stderr = "command timed out"
		} else {
			stderr = stderr + "\ncommand timed out"
		}
		return stdout, stderr, fmt.Errorf("docker command timeout")
	}
	if err != nil {
		if strings.TrimSpace(stderr) == "" {
			stderr = err.Error()
		}
		return stdout, stderr, fmt.Errorf("docker %s: %w", strings.Join(args, " "), err)
	}
	return stdout, stderr, nil
}

func clampTimeout(v int, def int, max int) int {
	if v <= 0 {
		v = def
	}
	if v > max {
		v = max
	}
	return v
}

func sanitizeSessionID(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, "_", "-")
	var b strings.Builder
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	clean := strings.ToLower(b.String())
	if len(clean) > 48 {
		clean = clean[:48]
	}
	return strings.Trim(clean, "-")
}

func extractExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}
