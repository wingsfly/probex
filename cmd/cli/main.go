package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var serverURL = "http://localhost:8080"

func main() {
	root := &cobra.Command{
		Use:   "probex",
		Short: "ProbeX - Network Quality Monitoring CLI",
	}
	root.PersistentFlags().StringVar(&serverURL, "server", "http://localhost:8080", "controller server URL")

	root.AddCommand(taskCmd())
	root.AddCommand(resultCmd())
	root.AddCommand(agentCmd())
	root.AddCommand(exportCmd())
	root.AddCommand(pluginCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// --- Task commands ---

func taskCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "task", Short: "Manage probe tasks"}

	create := &cobra.Command{
		Use:   "create",
		Short: "Create a probe task",
		RunE:  taskCreate,
	}
	create.Flags().String("name", "", "task name")
	create.Flags().String("target", "", "target host/URL")
	create.Flags().String("type", "icmp", "probe type: icmp, tcp, http, dns")
	create.Flags().String("interval", "30s", "probe interval")
	create.Flags().String("timeout", "10s", "probe timeout")
	create.Flags().String("config", "{}", "probe config JSON")

	cmd.AddCommand(create)
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List tasks", RunE: taskList})
	cmd.AddCommand(&cobra.Command{Use: "get [id]", Short: "Get task detail", Args: cobra.ExactArgs(1), RunE: taskGet})
	cmd.AddCommand(&cobra.Command{Use: "delete [id]", Short: "Delete task", Args: cobra.ExactArgs(1), RunE: taskDelete})
	cmd.AddCommand(&cobra.Command{Use: "pause [id]", Short: "Pause task", Args: cobra.ExactArgs(1), RunE: taskPause})
	cmd.AddCommand(&cobra.Command{Use: "resume [id]", Short: "Resume task", Args: cobra.ExactArgs(1), RunE: taskResume})
	cmd.AddCommand(&cobra.Command{Use: "run [id]", Short: "Run task once", Args: cobra.ExactArgs(1), RunE: taskRun})

	return cmd
}

func taskCreate(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	target, _ := cmd.Flags().GetString("target")
	probeType, _ := cmd.Flags().GetString("type")
	interval, _ := cmd.Flags().GetString("interval")
	timeout, _ := cmd.Flags().GetString("timeout")
	cfgStr, _ := cmd.Flags().GetString("config")

	if name == "" || target == "" {
		return fmt.Errorf("--name and --target are required")
	}

	body := map[string]any{
		"name":       name,
		"target":     target,
		"probe_type": probeType,
		"interval":   interval,
		"timeout":    timeout,
	}
	if cfgStr != "{}" {
		var cfg json.RawMessage
		if err := json.Unmarshal([]byte(cfgStr), &cfg); err != nil {
			return fmt.Errorf("invalid config json: %w", err)
		}
		body["config"] = cfg
	}

	resp, err := doRequest("POST", "/api/v1/tasks", body)
	if err != nil {
		return err
	}
	printJSON(resp)
	return nil
}

func taskList(cmd *cobra.Command, args []string) error {
	resp, err := doRequest("GET", "/api/v1/tasks", nil)
	if err != nil {
		return err
	}
	data, ok := resp["data"].([]any)
	if !ok || len(data) == 0 {
		fmt.Println("No tasks found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tTARGET\tTYPE\tINTERVAL\tENABLED")
	for _, item := range data {
		t := item.(map[string]any)
		id := truncID(t["id"])
		intervalNs := t["interval"].(float64)
		interval := time.Duration(intervalNs).String()
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%v\n",
			id, t["name"], t["target"], t["probe_type"], interval, t["enabled"])
	}
	w.Flush()
	return nil
}

func taskGet(cmd *cobra.Command, args []string) error {
	resp, err := doRequest("GET", "/api/v1/tasks/"+args[0], nil)
	if err != nil {
		return err
	}
	printJSON(resp)
	return nil
}

func taskDelete(cmd *cobra.Command, args []string) error {
	resp, err := doRequest("DELETE", "/api/v1/tasks/"+args[0], nil)
	if err != nil {
		return err
	}
	printJSON(resp)
	return nil
}

func taskPause(cmd *cobra.Command, args []string) error {
	resp, err := doRequest("POST", "/api/v1/tasks/"+args[0]+"/pause", nil)
	if err != nil {
		return err
	}
	printJSON(resp)
	return nil
}

func taskResume(cmd *cobra.Command, args []string) error {
	resp, err := doRequest("POST", "/api/v1/tasks/"+args[0]+"/resume", nil)
	if err != nil {
		return err
	}
	printJSON(resp)
	return nil
}

func taskRun(cmd *cobra.Command, args []string) error {
	resp, err := doRequest("POST", "/api/v1/tasks/"+args[0]+"/run", nil)
	if err != nil {
		return err
	}
	printJSON(resp)
	return nil
}

// --- Result commands ---

func resultCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "result", Short: "Query probe results"}

	query := &cobra.Command{
		Use:   "query",
		Short: "Query results",
		RunE:  resultQuery,
	}
	query.Flags().String("task-id", "", "filter by task ID")
	query.Flags().String("from", "", "start time (RFC3339)")
	query.Flags().String("to", "", "end time (RFC3339)")
	query.Flags().Int("limit", 20, "result limit")

	summary := &cobra.Command{
		Use:   "summary",
		Short: "Get result summary",
		RunE:  resultSummary,
	}
	summary.Flags().String("task-id", "", "filter by task ID")
	summary.Flags().String("from", "", "start time (RFC3339)")
	summary.Flags().String("to", "", "end time (RFC3339)")

	cmd.AddCommand(query)
	cmd.AddCommand(summary)
	cmd.AddCommand(&cobra.Command{Use: "latest", Short: "Latest results per task", RunE: resultLatest})

	return cmd
}

func resultQuery(cmd *cobra.Command, args []string) error {
	params := url.Values{}
	if v, _ := cmd.Flags().GetString("task-id"); v != "" {
		params.Set("task_id", v)
	}
	if v, _ := cmd.Flags().GetString("from"); v != "" {
		params.Set("from", v)
	}
	if v, _ := cmd.Flags().GetString("to"); v != "" {
		params.Set("to", v)
	}
	if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
		params.Set("limit", fmt.Sprintf("%d", v))
	}

	resp, err := doRequest("GET", "/api/v1/results?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	data, ok := resp["data"].([]any)
	if !ok || len(data) == 0 {
		fmt.Println("No results found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TIMESTAMP\tTASK\tSUCCESS\tLATENCY\tJITTER\tLOSS%\tERROR")
	for _, item := range data {
		r := item.(map[string]any)
		ts := r["timestamp"].(string)
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			ts = t.Format("15:04:05")
		}
		fmt.Fprintf(w, "%s\t%s\t%v\t%.1fms\t%s\t%s\t%s\n",
			ts, truncID(r["task_id"]), r["success"],
			r["latency_ms"].(float64),
			fmtOpt(r["jitter_ms"]), fmtOpt(r["packet_loss_pct"]),
			fmtStr(r["error"]))
	}
	w.Flush()
	return nil
}

func resultSummary(cmd *cobra.Command, args []string) error {
	params := url.Values{}
	if v, _ := cmd.Flags().GetString("task-id"); v != "" {
		params.Set("task_id", v)
	}
	if v, _ := cmd.Flags().GetString("from"); v != "" {
		params.Set("from", v)
	}
	if v, _ := cmd.Flags().GetString("to"); v != "" {
		params.Set("to", v)
	}

	resp, err := doRequest("GET", "/api/v1/results/summary?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	printJSON(resp)
	return nil
}

func resultLatest(cmd *cobra.Command, args []string) error {
	resp, err := doRequest("GET", "/api/v1/results/latest", nil)
	if err != nil {
		return err
	}
	printJSON(resp)
	return nil
}

// --- Agent commands ---

func agentCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "agent", Short: "Manage agents"}
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List agents", RunE: agentList})
	return cmd
}

func agentList(cmd *cobra.Command, args []string) error {
	resp, err := doRequest("GET", "/api/v1/agents", nil)
	if err != nil {
		return err
	}
	printJSON(resp)
	return nil
}

// --- Export commands ---

func exportCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "export", Short: "Export probe results"}

	csvCmd := &cobra.Command{
		Use:   "csv",
		Short: "Export results as CSV",
		RunE:  exportCSV,
	}
	csvCmd.Flags().String("task-id", "", "filter by task ID")
	csvCmd.Flags().String("from", "", "start time (RFC3339)")
	csvCmd.Flags().String("to", "", "end time (RFC3339)")
	csvCmd.Flags().StringP("output", "o", "", "output file (default: stdout)")

	cmd.AddCommand(csvCmd)
	return cmd
}

func exportCSV(cmd *cobra.Command, args []string) error {
	params := url.Values{}
	if v, _ := cmd.Flags().GetString("task-id"); v != "" {
		params.Set("task_id", v)
	}
	if v, _ := cmd.Flags().GetString("from"); v != "" {
		params.Set("from", v)
	}
	if v, _ := cmd.Flags().GetString("to"); v != "" {
		params.Set("to", v)
	}

	httpResp, err := http.Get(serverURL + "/api/v1/export/csv?" + params.Encode())
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	output, _ := cmd.Flags().GetString("output")
	var w io.Writer = os.Stdout
	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	io.Copy(w, httpResp.Body)
	return nil
}

// --- Plugin commands ---

func pluginCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "plugin", Short: "Manage plugins"}
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List available plugins", RunE: pluginList})
	return cmd
}

func pluginList(cmd *cobra.Command, args []string) error {
	resp, err := doRequest("GET", "/api/v1/plugins", nil)
	if err != nil {
		return err
	}
	printJSON(resp)
	return nil
}

// --- Helpers ---

func doRequest(method, path string, body any) (map[string]any, error) {
	var reqBody io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, serverURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

func printJSON(v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

func truncID(v any) string {
	s := fmt.Sprintf("%v", v)
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func fmtOpt(v any) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%.1f", v.(float64))
}

func fmtStr(v any) string {
	if v == nil {
		return ""
	}
	s := fmt.Sprintf("%v", v)
	s = strings.TrimSpace(s)
	return s
}
