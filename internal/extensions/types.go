package extensions

const (
	StatusMissing  = "missing"
	StatusStopped  = "stopped"
	StatusStarting = "starting"
	StatusRunning  = "running"
	StatusError    = "error"
)

type Status struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Description         string `json:"description"`
	BuiltIn             bool   `json:"built_in"`
	Installed           bool   `json:"installed"`
	Enabled             bool   `json:"enabled"`
	Status              string `json:"status"`
	BinaryPath          string `json:"binary_path"`
	ConfigPath          string `json:"config_path"`
	Port                int    `json:"port"`
	ProxyURL            string `json:"proxy_url"`
	ManagementURL       string `json:"management_url"`
	ManagementPath      string `json:"management_path"`
	ManagementInstalled bool   `json:"management_installed"`
	LastError           string `json:"last_error"`
}

type CLIProxyAPIOptions struct {
	PluginRoot string
	DataRoot   string
	Port       int
	LogLines   int
}
