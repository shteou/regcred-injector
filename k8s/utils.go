package k8s

type RegCredPatchSpec struct {
	Op    string              `json:"op"`
	Path  string              `json:"path"`
	Value []map[string]string `json:"value"`
}

type DockerAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Auth     string `json:"auth"`
}

type DockerConfig struct {
	Auths map[string]DockerAuth `json:"auths":`
}
