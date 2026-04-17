package guard

import "strings"

var kubectlFlagsWithValue = map[string]struct{}{
	"--context":               {},
	"--namespace":             {},
	"-n":                      {},
	"--kubeconfig":            {},
	"-s":                      {},
	"--server":                {},
	"--certificate-authority": {},
	"--client-certificate":    {},
	"--client-key":            {},
	"--token":                 {},
	"--as":                    {},
	"--as-group":              {},
	"--as-uid":                {},
	"--username":              {},
	"--password":              {},
	"--cache-dir":             {},
	"--request-timeout":       {},
	"--log-file":              {},
	"--v":                     {},
	"-v":                      {},
	"--cluster":               {},
	"--user":                  {},
	"--tls-server-name":       {},
}

func ExtractVerb(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			return a
		}
		if strings.Contains(a, "=") {
			continue
		}
		if _, ok := kubectlFlagsWithValue[a]; ok {
			i++
			continue
		}
	}
	return ""
}
