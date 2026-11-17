package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

type promClient struct {
	baseURL string
	http    *http.Client
}

func newPromClient(baseURL string) *promClient {
	return &promClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

type promResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Value  [2]interface{}    `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

// promSample is one instant-vector result row: its full label set plus
// the sampled value.
type promSample struct {
	Metric map[string]string
	Value  float64
}

// queryRaw runs an instant PromQL query and returns each result row with
// its full label set intact (unlike query, which collapses to instance->value).
func (p *promClient) queryRaw(promql string) ([]promSample, error) {
	u, err := url.Parse(p.baseURL + "/api/v1/query")
	if err != nil {
		return nil, fmt.Errorf("PROMETHEUS_URL tidak valid: %w", err)
	}
	q := u.Query()
	q.Set("query", promql)
	u.RawQuery = q.Encode()

	resp, err := p.http.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("request gagal: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gagal baca response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var pr promResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("gagal decode JSON: %w", err)
	}
	if pr.Status != "success" {
		return nil, fmt.Errorf("query error: %s", pr.Error)
	}

	samples := make([]promSample, 0, len(pr.Data.Result))
	for _, r := range pr.Data.Result {
		valStr, ok := r.Value[1].(string)
		if !ok {
			continue
		}
		v, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}
		samples = append(samples, promSample{Metric: r.Metric, Value: v})
	}
	return samples, nil
}

// query runs an instant PromQL query and returns a map keyed by the
// "instance" label. Queries used in this program always group by
// instance, so any result without that label is skipped.
func (p *promClient) query(promql string) (map[string]float64, error) {
	samples, err := p.queryRaw(promql)
	if err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(samples))
	for _, s := range samples {
		instance := s.Metric["instance"]
		if instance == "" {
			continue
		}
		out[instance] = s.Value
	}
	return out, nil
}

// gib is bytes per gibibyte, used to convert node_exporter's raw byte
// counters into the GB figure shown in the report.
const gib = 1024 * 1024 * 1024

// instanceMetrics is one row (one node_exporter target) in a client's report.
type instanceMetrics struct {
	Instance        string
	Job             string
	Up              bool
	CPU             float64
	Mem             float64
	DiskUsedPercent float64
	DiskFreeGB      float64
	HasStats        bool
}

// fetchClientMetrics queries the four report metrics for one client,
// filtered only by the client's federation label, and merges them into
// one row per instance sorted alphabetically.
//
// Deliberately NOT filtered by job: each client's own Prometheus names
// its node_exporter scrape job however it wants (e.g. "app-database-prod"
// instead of a generic "node"), and honor_labels:true on the central
// federation job preserves that original name. There is no single job
// name that works across all clients, so the client label — which IS
// guaranteed unique via external_labels — is the only reliable filter.
func fetchClientMetrics(p *promClient, clientLabel string) ([]instanceMetrics, error) {
	upQ := fmt.Sprintf(`up{client=%q}`, clientLabel)
	cpuQ := fmt.Sprintf(`100 - (avg by (instance) (rate(node_cpu_seconds_total{client=%q, mode="idle"}[5m])) * 100)`, clientLabel)
	memQ := fmt.Sprintf(`(1 - (node_memory_MemAvailable_bytes{client=%q} / node_memory_MemTotal_bytes{client=%q})) * 100`, clientLabel, clientLabel)
	// Avail/size queried as raw bytes (not a combined percent formula) so
	// we can report both the used-% and the remaining GB from the same values.
	diskAvailQ := fmt.Sprintf(`node_filesystem_avail_bytes{client=%q, fstype!~"tmpfs|overlay", mountpoint="/"}`, clientLabel)
	diskSizeQ := fmt.Sprintf(`node_filesystem_size_bytes{client=%q, fstype!~"tmpfs|overlay", mountpoint="/"}`, clientLabel)

	upSamples, err := p.queryRaw(upQ)
	if err != nil {
		return nil, fmt.Errorf("query 'up' gagal: %w", err)
	}
	if len(upSamples) == 0 {
		return nil, fmt.Errorf("tidak ada instance ditemukan untuk client=%q (cek external_labels di Prometheus client & federasi ke pusat)", clientLabel)
	}

	type upInfo struct {
		Value float64
		Job   string
	}
	upRes := make(map[string]upInfo, len(upSamples))
	for _, s := range upSamples {
		instance := s.Metric["instance"]
		if instance == "" {
			continue
		}
		upRes[instance] = upInfo{Value: s.Value, Job: s.Metric["job"]}
	}

	cpuRes, err := p.query(cpuQ)
	if err != nil {
		return nil, fmt.Errorf("query CPU gagal: %w", err)
	}
	memRes, err := p.query(memQ)
	if err != nil {
		return nil, fmt.Errorf("query memory gagal: %w", err)
	}
	diskAvailRes, err := p.query(diskAvailQ)
	if err != nil {
		return nil, fmt.Errorf("query disk avail gagal: %w", err)
	}
	diskSizeRes, err := p.query(diskSizeQ)
	if err != nil {
		return nil, fmt.Errorf("query disk size gagal: %w", err)
	}

	instances := make([]string, 0, len(upRes))
	for inst := range upRes {
		instances = append(instances, inst)
	}
	sort.Strings(instances)

	result := make([]instanceMetrics, 0, len(instances))
	for _, inst := range instances {
		m := instanceMetrics{Instance: inst, Job: upRes[inst].Job, Up: upRes[inst].Value == 1}
		if m.Up {
			cpu, okC := cpuRes[inst]
			mem, okM := memRes[inst]
			avail, okA := diskAvailRes[inst]
			size, okS := diskSizeRes[inst]
			if okC && okM && okA && okS && size > 0 {
				m.CPU, m.Mem = cpu, mem
				m.DiskUsedPercent = (1 - avail/size) * 100
				m.DiskFreeGB = avail / gib
				m.HasStats = true
			}
		}
		result = append(result, m)
	}
	return result, nil
}
