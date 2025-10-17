package config

type Config struct {
	GamesRoot string `json:"gamesRoot"`
	Debug     bool   `json:"debug"`
}

func NewConfig() Config {
	return Config{}
}
