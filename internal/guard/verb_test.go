package guard

import "testing"

func TestExtractVerb(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"simple delete", []string{"delete", "pod", "my-pod"}, "delete"},
		{"get command", []string{"get", "pods"}, "get"},
		{"empty args", []string{}, ""},
		{"only flags", []string{"--help"}, ""},
		{"context flag before verb", []string{"--context=prod", "delete", "pod"}, "delete"},
		{"context flag separate value", []string{"--context", "prod", "delete", "pod"}, "delete"},
		{"namespace short flag", []string{"-n", "kube-system", "delete", "pod"}, "delete"},
		{"mixed flags", []string{"--kubeconfig", "/tmp/cfg", "-n", "default", "delete", "pod"}, "delete"},
		{"kv form only", []string{"--namespace=default", "--context=prod", "apply", "-f", "x.yaml"}, "apply"},
		{"v flag with value", []string{"-v", "6", "drain", "node-1"}, "drain"},
		{"unknown flag ignored", []string{"--some-future-flag", "get", "pods"}, "get"},
		{"help alone", []string{"help", "delete"}, "help"},
		{"global before subcommand", []string{"--as", "admin", "--as-group", "system:masters", "delete", "ns", "kube-system"}, "delete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractVerb(tt.args)
			if got != tt.want {
				t.Errorf("ExtractVerb(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}
