package config

type Config struct {
	GamesRoot                  string `json:"gamesRoot"`
	Debug                      bool   `json:"debug"`
	RetroBatPath               string `json:"retrobatPath"`
	DownloadControllerDatabase bool   `json:"downloadControllerDatabase"`
}

func NewConfig() Config {
	return Config{}
}
