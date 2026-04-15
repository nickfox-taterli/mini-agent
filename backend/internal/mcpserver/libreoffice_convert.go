package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------------------------------------------------------------------------
// LibreOffice 运行时配置
// ---------------------------------------------------------------------------

type loConfig struct {
	DockerImage           string
	DefaultTimeoutSeconds int
	MaxTimeoutSeconds     int
	MemoryLimit           string
	CPULimit              float64
	PidsLimit             int
}

var loCfg *loConfig

// InitLibreOfficeConfig 在 main.go 启动时调用, 设置 LibreOffice 工具配置.
func InitLibreOfficeConfig(dockerImage string, defaultTimeout, maxTimeout int, memoryLimit string, cpuLimit float64, pidsLimit int) {
	loCfg = &loConfig{
		DockerImage:           dockerImage,
		DefaultTimeoutSeconds: defaultTimeout,
		MaxTimeoutSeconds:     maxTimeout,
		MemoryLimit:           memoryLimit,
		CPULimit:              cpuLimit,
		PidsLimit:             pidsLimit,
	}
	log.Printf("[libreoffice] initialized image=%s timeout=%ds/%ds memory=%s",
		dockerImage, defaultTimeout, maxTimeout, memoryLimit)
}

func getLOConfig() *loConfig {
	if loCfg != nil {
		return loCfg
	}
	return &loConfig{
		DockerImage:           "lscr.io/linuxserver/libreoffice:latest",
		DefaultTimeoutSeconds: 120,
		MaxTimeoutSeconds:     600,
		MemoryLimit:           "512m",
		CPULimit:              1.0,
		PidsLimit:             64,
	}
}

// ---------------------------------------------------------------------------
// 共享辅助函数
// ---------------------------------------------------------------------------

const loHeavyweightPrefix = "Heavyweight operation - launches a Docker container (~1GB image). Prefer existing Skills (minimax-xlsx, minimax-docx, minimax-pdf, pptx-generator) when possible. "

// loAllowedOutputFormats 定义允许的输出格式白名单.
var loAllowedOutputFormats = map[string]string{
	"pdf":  "pdf",
	"html": "html",
	"htm":  "html",
	"txt":  "txt",
	"csv":  "csv",
	"png":  "png",
	"jpg":  "jpg",
	"jpeg": "jpg",
	"odt":  "odt",
	"ods":  "ods",
	"odp":  "odp",
	"xlsx": "xlsx",
	"docx": "docx",
	"pptx": "pptx",
	"rtf":  "rtf",
}

func loNormalizeFormat(format string) (string, error) {
	f := strings.ToLower(strings.TrimSpace(format))
	if f == "" {
		return "pdf", nil
	}
	normalized, ok := loAllowedOutputFormats[f]
	if !ok {
		allowed := make([]string, 0, len(loAllowedOutputFormats))
		for k := range loAllowedOutputFormats {
			allowed = append(allowed, k)
		}
		return "", fmt.Errorf("unsupported output format: %s (allowed: %s)", f, strings.Join(allowed, ", "))
	}
	return normalized, nil
}

// loValidateSourceFile 验证源文件存在且路径合法.
func loValidateSourceFile(path string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("source file not found: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("source path is a directory, not a file")
	}
	return abs, nil
}

// loValidateSourceDirectory 验证源目录存在且路径合法.
func loValidateSourceDirectory(path string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("source directory not found: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("source path is a file, not a directory")
	}
	return abs, nil
}

// loRunContainer 执行一次性 Docker 容器命令.
func loRunContainer(timeout int, mounts []string, env []string, cmdArgs []string) (string, string, int64, error) {
	cfg := getLOConfig()
	timeout = clampTimeout(timeout, cfg.DefaultTimeoutSeconds, cfg.MaxTimeoutSeconds)

	dockerBin, err := exec.LookPath("docker")
	if err != nil {
		return "", "", 0, fmt.Errorf("docker binary not found: %w", err)
	}

	// 注意: linuxserver/libreoffice 使用 s6-overlay init, 不兼容 --read-only.
	args := []string{
		"run", "--rm",
		"--security-opt", "no-new-privileges",
		"--pids-limit", fmt.Sprintf("%d", cfg.PidsLimit),
		"--memory", cfg.MemoryLimit,
		"--cpus", fmt.Sprintf("%.2f", cfg.CPULimit),
		"-e", "HOME=/tmp",
		"-e", "LANG=C.UTF-8",
	}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	args = append(args, mounts...)
	args = append(args, cfg.DockerImage)
	args = append(args, cmdArgs...)

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, dockerBin, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	duration := time.Since(start).Milliseconds()

	outStr := stdout.String()
	errStr := stderr.String()

	if ctx.Err() == context.DeadlineExceeded {
		return outStr, errStr + "\ncommand timed out", duration, fmt.Errorf("libreoffice container timed out after %ds", timeout)
	}
	if err != nil {
		return outStr, errStr, duration, fmt.Errorf("libreoffice container failed: %w\nstderr: %s", err, errStr)
	}
	return outStr, errStr, duration, nil
}

// loBuildConvertMountArgs 构建单文件转换的挂载参数 (同路径挂载).
func loBuildConvertMountArgs(sourceDir string, outDir string) []string {
	seen := map[string]bool{}
	var mounts []string
	addMount := func(hostPath, mode string) {
		if seen[hostPath] {
			return
		}
		seen[hostPath] = true
		mounts = append(mounts, "-v", hostPath+":"+hostPath+":"+mode)
	}

	if sourceDir == outDir {
		// 同目录时只需一个 rw 挂载
		addMount(outDir, "rw")
	} else {
		addMount(sourceDir, "ro")
		addMount(outDir, "rw")
	}

	// 若源目录不在 frontend/upload 下, 额外挂载 upload root
	if uploadRoot, err := resolveFrontendUploadRootDir(); err == nil {
		absRoot, _ := filepath.Abs(uploadRoot)
		if absRoot != "" && !strings.HasPrefix(sourceDir, absRoot) && !strings.HasPrefix(outDir, absRoot) {
			addMount(absRoot, "ro")
		}
	}
	return mounts
}

// loBuildBatchMountArgs 构建批量转换的挂载参数.
func loBuildBatchMountArgs(sourceDir string, outDir string) []string {
	return loBuildConvertMountArgs(sourceDir, outDir)
}

// ---------------------------------------------------------------------------
// 工具 1: libreoffice_convert - 文档格式转换
// ---------------------------------------------------------------------------

type libreofficeConvertInput struct {
	FilePath     string `json:"file_path" jsonschema:"Absolute path to the source document (e.g. .docx, .xlsx, .pptx, .odt)"`
	OutputFormat string `json:"output_format,omitempty" jsonschema:"Target format extension: pdf (default), html, txt, csv, png, jpg, odt, ods, odp, xlsx, docx, pptx, rtf"`
}

type libreofficeConvertOutput struct {
	SourcePath string `json:"source_path"`
	OutputPath string `json:"output_path"`
	OutputURL  string `json:"output_url"`
	OutputName string `json:"output_name"`
	SizeBytes  int64  `json:"size_bytes"`
	DurationMs int64  `json:"duration_ms"`
}

func libreofficeConvert(_ context.Context, _ *mcp.CallToolRequest, in libreofficeConvertInput) (*mcp.CallToolResult, libreofficeConvertOutput, error) {
	out, err := libreofficeConvertLocal(in)
	if err != nil {
		return nil, libreofficeConvertOutput{}, err
	}
	return nil, out, nil
}

func libreofficeConvertLocal(in libreofficeConvertInput) (libreofficeConvertOutput, error) {
	absPath, err := loValidateSourceFile(in.FilePath)
	if err != nil {
		return libreofficeConvertOutput{}, err
	}

	format, err := loNormalizeFormat(in.OutputFormat)
	if err != nil {
		return libreofficeConvertOutput{}, err
	}

	outDir, err := resolveFrontendUploadDir()
	if err != nil {
		return libreofficeConvertOutput{}, fmt.Errorf("resolve output dir: %w", err)
	}

	cfg := getLOConfig()
	if err := ensureLOImage(cfg.DockerImage); err != nil {
		return libreofficeConvertOutput{}, fmt.Errorf("ensure libreoffice image: %w", err)
	}

	sourceDir := filepath.Dir(absPath)
	mounts := loBuildConvertMountArgs(sourceDir, outDir)
	cmdArgs := []string{"libreoffice", "--headless", "--convert-to", format, "--outdir", outDir, absPath}

	_, _, durationMs, err := loRunContainer(0, mounts, nil, cmdArgs)
	if err != nil {
		return libreofficeConvertOutput{}, err
	}

	// 定位输出文件
	ext := filepath.Ext(absPath)
	baseName := strings.TrimSuffix(filepath.Base(absPath), ext)
	outputName := baseName + "." + format
	outputPath := filepath.Join(outDir, outputName)

	stat, err := os.Stat(outputPath)
	if err != nil {
		return libreofficeConvertOutput{}, fmt.Errorf("output file not found at %s: %w", outputPath, err)
	}

	outputURL := BuildFileURL(outDir, outputName)

	return libreofficeConvertOutput{
		SourcePath: absPath,
		OutputPath: outputPath,
		OutputURL:  outputURL,
		OutputName: outputName,
		SizeBytes:  stat.Size(),
		DurationMs: durationMs,
	}, nil
}

// ---------------------------------------------------------------------------
// 工具 2: libreoffice_extract_text - 文档文本提取
// ---------------------------------------------------------------------------

type libreofficeExtractTextInput struct {
	FilePath string `json:"file_path" jsonschema:"Absolute path to the document to extract text from (docx, xlsx, pptx, odt, etc.)"`
}

type libreofficeExtractTextOutput struct {
	SourcePath  string `json:"source_path"`
	TextContent string `json:"text_content"`
	CharCount   int    `json:"char_count"`
	DurationMs  int64  `json:"duration_ms"`
}

func libreofficeExtractText(_ context.Context, _ *mcp.CallToolRequest, in libreofficeExtractTextInput) (*mcp.CallToolResult, libreofficeExtractTextOutput, error) {
	out, err := libreofficeExtractTextLocal(in)
	if err != nil {
		return nil, libreofficeExtractTextOutput{}, err
	}
	return nil, out, nil
}

func libreofficeExtractTextLocal(in libreofficeExtractTextInput) (libreofficeExtractTextOutput, error) {
	absPath, err := loValidateSourceFile(in.FilePath)
	if err != nil {
		return libreofficeExtractTextOutput{}, err
	}

	cfg := getLOConfig()
	if err := ensureLOImage(cfg.DockerImage); err != nil {
		return libreofficeExtractTextOutput{}, fmt.Errorf("ensure libreoffice image: %w", err)
	}

	outDir, err := resolveFrontendUploadDir()
	if err != nil {
		return libreofficeExtractTextOutput{}, fmt.Errorf("resolve output dir: %w", err)
	}

	sourceDir := filepath.Dir(absPath)
	mounts := loBuildConvertMountArgs(sourceDir, outDir)

	ext := strings.ToLower(filepath.Ext(absPath))
	baseName := strings.TrimSuffix(filepath.Base(absPath), filepath.Ext(absPath))

	// Calc 文档 (xlsx/xls/ods) 没有 txt 导出过滤器, 使用 html 作为中间格式.
	// 其他类型优先尝试 txt, 失败后回退 html.
	isSpreadsheet := ext == ".xlsx" || ext == ".xls" || ext == ".ods" || ext == ".csv"

	var text string
	var durationMs int64

	if isSpreadsheet {
		text, durationMs, err = loExtractViaHTML(mounts, absPath, outDir, baseName)
	} else {
		// 先尝试 txt
		text, durationMs, err = loExtractViaTXT(mounts, absPath, outDir, baseName)
		if err != nil {
			// txt 失败, 回退到 html
			log.Printf("[libreoffice] txt extraction failed for %s, falling back to html: %v", absPath, err)
			text, durationMs, err = loExtractViaHTML(mounts, absPath, outDir, baseName)
		}
	}
	if err != nil {
		return libreofficeExtractTextOutput{}, err
	}

	return libreofficeExtractTextOutput{
		SourcePath:  absPath,
		TextContent: text,
		CharCount:   len(text),
		DurationMs:  durationMs,
	}, nil
}

// loExtractViaTXT 使用 --convert-to txt 提取纯文本.
func loExtractViaTXT(mounts []string, absPath, outDir, baseName string) (string, int64, error) {
	// 使用唯一临时目录避免并发冲突
	tmpDir, err := os.MkdirTemp(outDir, "lo-extract-")
	if err != nil {
		return "", 0, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	allMounts := append(mounts, "-v", tmpDir+":"+tmpDir+":rw")

	cmdArgs := []string{"libreoffice", "--headless", "--convert-to", "txt:Text", "--outdir", tmpDir, absPath}
	stdout, stderr, durationMs, err := loRunContainer(0, allMounts, nil, cmdArgs)
	if err != nil {
		return "", durationMs, fmt.Errorf("extract via txt failed: %w\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	txtPath := filepath.Join(tmpDir, baseName+".txt")
	data, err := os.ReadFile(txtPath)
	if err != nil {
		return "", durationMs, fmt.Errorf("read txt output: %w", err)
	}
	return string(data), durationMs, nil
}

// loExtractViaHTML 使用 --convert-to html 提取文本, 然后去除 HTML 标签.
// 为避免并发冲突, 将源文件复制到唯一临时目录再转换.
func loExtractViaHTML(mounts []string, absPath, outDir, baseName string) (string, int64, error) {
	// 使用唯一临时目录避免并发冲突
	tmpDir, err := os.MkdirTemp(outDir, "lo-extract-")
	if err != nil {
		return "", 0, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// 额外挂载临时目录
	allMounts := append(mounts, "-v", tmpDir+":"+tmpDir+":rw")

	cmdArgs := []string{"libreoffice", "--headless", "--convert-to", "html", "--outdir", tmpDir, absPath}
	stdout, stderr, durationMs, err := loRunContainer(0, allMounts, nil, cmdArgs)
	if err != nil {
		return "", durationMs, fmt.Errorf("extract via html failed: %w\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	htmlPath := filepath.Join(tmpDir, baseName+".html")
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		return "", durationMs, fmt.Errorf("read html output: %w", err)
	}

	text := loStripHTMLTags(string(data))
	return text, durationMs, nil
}

// loStripHTMLTags 去除 HTML 标签, 提取纯文本.
func loStripHTMLTags(html string) string {
	// 先移除 <style>...</style> 和 <script>...</script> 整个块
	result := html
	for {
		start := strings.Index(result, "<style")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "</style>")
		if end == -1 {
			result = result[:start]
			break
		}
		result = result[:start] + result[start+end+len("</style>"):]
	}
	for {
		start := strings.Index(result, "<script")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "</script>")
		if end == -1 {
			result = result[:start]
			break
		}
		result = result[:start] + result[start+end+len("</script>"):]
	}

	// 替换常见结构性标签为换行符
	re := strings.NewReplacer(
		"<br>", "\n", "<br/>", "\n", "<br />", "\n",
		"<p>", "\n", "</p>", "\n",
		"<tr>", "\n", "</tr>", "",
		"<td>", "\t", "</td>", " ",
		"<th>", "\t", "</th>", " ",
		"<li>", "- ", "</li>", "\n",
		"<div>", "\n", "</div>", "\n",
		"<h1>", "\n", "</h1>", "\n",
		"<h2>", "\n", "</h2>", "\n",
		"<h3>", "\n", "</h3>", "\n",
	)
	result = re.Replace(result)

	// 移除所有 <...> 标签
	var buf strings.Builder
	inTag := false
	for _, r := range result {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			buf.WriteRune(r)
		}
	}

	// 解码常见 HTML 实体
	text := buf.String()
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")

	// 清理多余空白行, 只保留非空行
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return strings.Join(cleaned, "\n")
}

// ---------------------------------------------------------------------------
// 工具 3: libreoffice_batch_convert - 批量格式转换
// ---------------------------------------------------------------------------

type libreofficeBatchConvertInput struct {
	Directory    string `json:"directory" jsonschema:"Absolute path to directory containing source files"`
	OutputFormat string `json:"output_format,omitempty" jsonschema:"Target format, default pdf"`
	FilePattern  string `json:"file_pattern,omitempty" jsonschema:"File glob pattern, default *.* (e.g. *.docx, *.xlsx, *.pptx)"`
}

type libreofficeBatchConvertResult struct {
	SourcePath string `json:"source_path"`
	OutputPath string `json:"output_path"`
	OutputURL  string `json:"output_url"`
	OutputName string `json:"output_name"`
	SizeBytes  int64  `json:"size_bytes"`
}

type libreofficeBatchConvertOutput struct {
	Results      []libreofficeBatchConvertResult `json:"results"`
	TotalCount   int                             `json:"total_count"`
	SuccessCount int                             `json:"success_count"`
	DurationMs   int64                           `json:"duration_ms"`
}

func libreofficeBatchConvert(_ context.Context, _ *mcp.CallToolRequest, in libreofficeBatchConvertInput) (*mcp.CallToolResult, libreofficeBatchConvertOutput, error) {
	out, err := libreofficeBatchConvertLocal(in)
	if err != nil {
		return nil, libreofficeBatchConvertOutput{}, err
	}
	return nil, out, nil
}

func libreofficeBatchConvertLocal(in libreofficeBatchConvertInput) (libreofficeBatchConvertOutput, error) {
	absDir, err := loValidateSourceDirectory(in.Directory)
	if err != nil {
		return libreofficeBatchConvertOutput{}, err
	}

	format, err := loNormalizeFormat(in.OutputFormat)
	if err != nil {
		return libreofficeBatchConvertOutput{}, err
	}

	pattern := strings.TrimSpace(in.FilePattern)
	if pattern == "" {
		pattern = "*"
	}

	cfg := getLOConfig()
	if err := ensureLOImage(cfg.DockerImage); err != nil {
		return libreofficeBatchConvertOutput{}, fmt.Errorf("ensure libreoffice image: %w", err)
	}

	outDir, err := resolveFrontendUploadDir()
	if err != nil {
		return libreofficeBatchConvertOutput{}, fmt.Errorf("resolve output dir: %w", err)
	}

	mounts := loBuildBatchMountArgs(absDir, outDir)

	// 通过 bash -c 执行, 以支持 glob 展开
	shellCmd := fmt.Sprintf("cd %s && libreoffice --headless --convert-to %s --outdir %s %s",
		absDir, format, outDir, filepath.Join(absDir, pattern))
	cmdArgs := []string{"bash", "-c", shellCmd}

	_, _, durationMs, err := loRunContainer(0, mounts, nil, cmdArgs)
	if err != nil {
		return libreofficeBatchConvertOutput{}, err
	}

	results, err := loCollectBatchResults(absDir, outDir, format)
	if err != nil {
		return libreofficeBatchConvertOutput{}, fmt.Errorf("collect batch results: %w", err)
	}

	return libreofficeBatchConvertOutput{
		Results:      results,
		TotalCount:   len(results),
		SuccessCount: len(results),
		DurationMs:   durationMs,
	}, nil
}

// loCollectBatchResults 扫描输出目录, 匹配源目录中文件名对应的转换结果.
func loCollectBatchResults(sourceDir string, outDir string, format string) ([]libreofficeBatchConvertResult, error) {
	var results []libreofficeBatchConvertResult

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if ext == "" || ext == "."+format {
			continue
		}
		lowerExt := strings.ToLower(ext)
		if !loIsDocumentExt(lowerExt) {
			continue
		}

		baseName := strings.TrimSuffix(name, ext)
		outputName := baseName + "." + format
		outputPath := filepath.Join(outDir, outputName)

		stat, err := os.Stat(outputPath)
		if err != nil {
			continue
		}

		results = append(results, libreofficeBatchConvertResult{
			SourcePath: filepath.Join(sourceDir, name),
			OutputPath: outputPath,
			OutputURL:  BuildFileURL(outDir, outputName),
			OutputName: outputName,
			SizeBytes:  stat.Size(),
		})
	}

	return results, nil
}

// loIsDocumentExt 判断扩展名是否为文档类型.
func loIsDocumentExt(ext string) bool {
	switch ext {
	case ".docx", ".doc", ".odt", ".rtf", ".xlsx", ".xls", ".ods", ".csv",
		".pptx", ".ppt", ".odp", ".pdf", ".html", ".htm", ".txt":
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// 工具 4: libreoffice_read_metadata - 读取文档属性
// ---------------------------------------------------------------------------

type libreofficeReadMetadataInput struct {
	FilePath string `json:"file_path" jsonschema:"Absolute path to the document"`
}

type libreofficeReadMetadataOutput struct {
	SourcePath string                 `json:"source_path"`
	Metadata   map[string]interface{} `json:"metadata"`
	DurationMs int64                  `json:"duration_ms"`
}

func libreofficeReadMetadata(_ context.Context, _ *mcp.CallToolRequest, in libreofficeReadMetadataInput) (*mcp.CallToolResult, libreofficeReadMetadataOutput, error) {
	out, err := libreofficeReadMetadataLocal(in)
	if err != nil {
		return nil, libreofficeReadMetadataOutput{}, err
	}
	return nil, out, nil
}

// loMetadataScript 是在容器内运行的元数据提取脚本.
// 优先尝试 file 命令获取基本信息, 若有 python3+uno 则读取详细元数据.
const loMetadataScript = `
import os, sys, json, subprocess

filepath = sys.argv[1]
stat = os.stat(filepath)
result = {
    "file_name": os.path.basename(filepath),
    "file_size": stat.st_size,
    "file_type": os.path.splitext(filepath)[1].lower(),
    "modified_time": stat.st_mtime,
}

# 使用 file 命令获取基本信息
try:
    r = subprocess.run(["file", filepath], capture_output=True, text=True, timeout=5)
    if r.returncode == 0:
        result["file_description"] = r.stdout.strip()
except:
    pass

# 尝试使用 LibreOffice UNO 获取详细元数据
try:
    import uno
    from com.sun.star.beans import PropertyValue

    local_ctx = uno.getComponentContext()
    resolver = local_ctx.ServiceManager.createInstanceWithContext(
        "com.sun.star.bridge.UnoUrlResolver", local_ctx)
    try:
        remote_ctx = resolver.resolve(
            "uno:socket,host=localhost,port=2002;urp;StarOffice.ComponentContext")
        smgr = remote_ctx.ServiceManager
        desktop = smgr.createInstanceWithContext("com.sun.star.frame.Desktop", remote_ctx)
    except:
        # 无需远程连接, 直接使用本地 UNO
        smgr = local_ctx.ServiceManager
        desktop = smgr.createInstanceWithContext("com.sun.star.frame.Desktop", local_ctx)

    url = "file://" + filepath
    pv = PropertyValue()
    pv.Name = "Hidden"
    pv.Value = True
    doc = desktop.loadComponentFromURL(url, "_blank", 0, (pv,))

    if doc is not None:
        try:
            info = doc.getDocumentInfo()
            result["title"] = info.getTitle() if hasattr(info, 'getTitle') else ""
            result["author"] = info.getAuthor() if hasattr(info, 'getAuthor') else ""
            result["subject"] = info.getSubject() if hasattr(info, 'getSubject') else ""
            result["keywords"] = str(info.getKeywords()) if hasattr(info, 'getKeywords') else ""
        except:
            pass

        try:
            stats = doc.getDocumentStatistics()
            for s in stats:
                result[s.Name] = s.Value
        except:
            pass

        doc.close(True)
    result["uno_available"] = True
except Exception as e:
    result["uno_available"] = False
    result["uno_error"] = str(e)

print(json.dumps(result, ensure_ascii=False))
`

func libreofficeReadMetadataLocal(in libreofficeReadMetadataInput) (libreofficeReadMetadataOutput, error) {
	absPath, err := loValidateSourceFile(in.FilePath)
	if err != nil {
		return libreofficeReadMetadataOutput{}, err
	}

	cfg := getLOConfig()
	if err := ensureLOImage(cfg.DockerImage); err != nil {
		return libreofficeReadMetadataOutput{}, fmt.Errorf("ensure libreoffice image: %w", err)
	}

	sourceDir := filepath.Dir(absPath)
	outDir, _ := resolveFrontendUploadDir()

	mounts := loBuildConvertMountArgs(sourceDir, outDir)

	// 通过 bash -c 传入 Python 脚本
	cmdArgs := []string{
		"bash", "-c",
		fmt.Sprintf("python3 -c %s %s", shellEscape(loMetadataScript), shellEscape(absPath)),
	}

	stdout, _, durationMs, err := loRunContainer(cfg.MaxTimeoutSeconds, mounts, nil, cmdArgs)
	if err != nil {
		return libreofficeReadMetadataOutput{}, err
	}

	var metadata map[string]interface{}
	if parseErr := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &metadata); parseErr != nil {
		metadata = map[string]interface{}{
			"raw_output": stdout,
		}
	}

	return libreofficeReadMetadataOutput{
		SourcePath: absPath,
		Metadata:   metadata,
		DurationMs: durationMs,
	}, nil
}

// shellEscape 对 shell 参数进行简单转义.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// ensureLOImage 确保 LibreOffice Docker 镜像存在.
func ensureLOImage(image string) error {
	if dockerRT != nil {
		return dockerRT.ensureImage(image)
	}
	dockerBin, err := exec.LookPath("docker")
	if err != nil {
		return fmt.Errorf("docker not found: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, dockerBin, "image", "inspect", image)
	if err := cmd.Run(); err != nil {
		log.Printf("[libreoffice] image %s not found locally, pulling...", image)
		pullCtx, pullCancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer pullCancel()
		pullCmd := exec.CommandContext(pullCtx, dockerBin, "pull", image)
		if out, err := pullCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("pull image %s: %w\noutput: %s", image, err, string(out))
		}
		log.Printf("[libreoffice] image %s pulled successfully", image)
	}
	return nil
}
