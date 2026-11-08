package worker

import "github.com/BurntSushi/toml"

// CheckConfig runs worker.toml's strict load over content without touching worker.toml. The error names the problem
// the way LoadConfig does, with the file called worker.toml.
func CheckConfig(content string) error {
	var c Config
	meta, err := toml.Decode(content, &c)
	return c.checkDecoded(meta, err, "worker.toml")
}
