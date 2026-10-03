package metrics

import (
	"sort"
	"strings"
)

func writeHeader(sb *strings.Builder, name, help, metricType string) {
	sb.WriteString("# HELP ")
	sb.WriteString(name)
	sb.WriteByte(' ')
	sb.WriteString(help)
	sb.WriteByte('\n')
	sb.WriteString("# TYPE ")
	sb.WriteString(name)
	sb.WriteByte(' ')
	sb.WriteString(metricType)
	sb.WriteByte('\n')
}

func encodeLabels(names, values []string) string {
	var sb strings.Builder
	for i, name := range names {
		if i > 0 {
			sb.WriteByte(0)
		}
		sb.WriteString(name)
		sb.WriteByte(0)
		if i < len(values) {
			sb.WriteString(values[i])
		}
	}
	return sb.String()
}

func parseLabelKey(key string) map[string]string {
	out := make(map[string]string)
	parts := strings.Split(key, "\x00")
	for i := 0; i+1 < len(parts); i += 2 {
		if parts[i] == "" {
			continue
		}
		out[parts[i]] = parts[i+1]
	}
	return out
}

func joinLabelNames(labels map[string]string) string {
	names := make([]string, 0, len(labels))
	for name := range labels {
		names = append(names, name)
	}
	sort.Strings(names)

	pairs := make([]string, 0, len(names))
	for _, name := range names {
		pairs = append(pairs, name+`="`+escapeLabelValue(labels[name])+`"`)
	}
	return strings.Join(pairs, ",")
}

func mergeLabels(labels map[string]string, name, value string) map[string]string {
	merged := make(map[string]string, len(labels)+1)
	for key, val := range labels {
		merged[key] = val
	}
	merged[name] = value
	return merged
}

func escapeLabelValue(v string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(v)
}
