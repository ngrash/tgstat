package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/ngrash/tgstat/backfill"
	"github.com/ngrash/tgstat/tgexport"
)

var (
	chatExportsGlob     = flag.String("chat-exports-glob", "chat-exports/*/result.json", "Glob pattern to find chat exports")
	aliasesFileFlag     = flag.String("aliases-file", "configs/aliases.json", "File with sender aliases")
	expressionsFileFlag = flag.String("expressions-file", "configs/expressions.json", "File with expressions to search for")
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	flag.Parse()

	files, err := filepath.Glob(*chatExportsGlob)
	if err != nil {
		return fmt.Errorf("find files: %w", err)
	}

	metrics, err := readAndAnalyzeChatExports(files)
	if err != nil {
		return fmt.Errorf("analyze chat exports: %w", err)
	}

	fmt.Println("Uploading to VictoriaMetrics")
	if err := uploadToVictoriaMetrics(metrics); err != nil {
		return fmt.Errorf("upload to VictoriaMetrics: %w", err)
	}

	fmt.Println("Done")

	return nil
}

func readAndAnalyzeChatExports(files []string) (*backfill.Metrics, error) {
	aliases, err := loadAliasFile(*aliasesFileFlag)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("%q: Alias file not found. Will not replace sender names.\n", *aliasesFileFlag)
		} else {
			return nil, fmt.Errorf("load aliases: %w", err)
		}
	}

	expressions, err := loadExpressionsFile(*expressionsFileFlag)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("%q: Expressions file not found. Will not search for expressions.\n", *expressionsFileFlag)
		} else {
			return nil, fmt.Errorf("load expressions: %w", err)
		}
	}

	metrics := backfill.NewMetrics()
	for _, in := range files {
		fmt.Println("Analyzing", in)
		data, err := tgexport.ReadFile(in)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}

		applySenderAliases(data, aliases)

		chatMetrics := metrics.With("file", in)

		if err := analyzeChat(data, chatMetrics, expressions); err != nil {
			return nil, fmt.Errorf("analyze %q: %w", in, err)
		}
	}
	return metrics, nil
}

func loadExpressionsFile(path string) ([]*regexp.Regexp, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var exprs []string
	if err := json.Unmarshal(buf, &exprs); err != nil {
		return nil, err
	}

	var compiled []*regexp.Regexp
	for _, expr := range exprs {
		r, err := regexp.Compile(expr)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, r)
	}
	return compiled, nil
}

type aliasMap map[tgexport.Sender]tgexport.Sender

func loadAliasFile(path string) (aliasMap, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var a aliasMap
	if err := json.Unmarshal(buf, &a); err != nil {
		return nil, err
	}
	return a, nil
}

func applySenderAliases(data *tgexport.Result, aliases aliasMap) {
	for i, m := range data.Messages {
		if alias, replace := aliases[m.From]; replace {
			data.Messages[i].From = alias
		}
	}
}

func victoriaMetricsURL() string {
	if url := os.Getenv("VICTORIAMETRICS_URL"); url != "" {
		return url
	}
	return "http://localhost:8428"
}

func uploadToVictoriaMetrics(metrics *backfill.Metrics) error {
	var compressed bytes.Buffer

	// Compress the metrics.
	w := gzip.NewWriter(&compressed)
	if err := metrics.Write(w, 1*time.Hour); err != nil {
		return fmt.Errorf("write metrics: %w", err)
	}
	// Flushing is important, otherwise the compressed data might not be complete.
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush gzip writer: %w", err)
	}

	// Delete the existing metrics.
	if err := deleteRemoteMetrics(); err != nil {
		return fmt.Errorf("delete remote metrics: %w", err)
	}

	// Upload the compressed metrics.
	req, err := http.NewRequest("POST", victoriaMetricsURL()+"/api/v1/import/prometheus", &compressed)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("response status: %s", resp.Status)
	}
	return nil
}

func deleteRemoteMetrics() error {
	resp, err := http.Get(fmt.Sprintf(victoriaMetricsURL()+"/api/v1/admin/tsdb/delete_series?match[]={__name__=~\"%s.*\"}", metricsPrefix))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("response status: %s", resp.Status)
	}
	return nil
}
