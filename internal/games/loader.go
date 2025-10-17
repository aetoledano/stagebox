package games

import (
	"encoding/base64"
	"launcher/tools"
	"log"
	"os"
	"path"

	"github.com/google/uuid"
)

func LoadGames(rootPath string) map[uuid.UUID]*GameData {
	rootEntries, err := os.ReadDir(rootPath)
	if err != nil {
		log.Fatal(err)
	}

	games := map[uuid.UUID]*GameData{}
	for _, e := range rootEntries {
		if !e.IsDir() {
			continue
		}

		lcPath := path.Join(rootPath, e.Name(), "lc.json")
		gd := NewGameData()
		err := tools.LoadJSONFile(lcPath, &gd)
		if err != nil {
			log.Printf("Error loading game data from %s: %v", lcPath, err)

			continue
		}
		gd.Image = path.Join(rootPath, e.Name(), gd.Image)
		gd.Executable = path.Join(rootPath, e.Name(), gd.Executable)
		gd.StartsOn = path.Join(rootPath, e.Name())
		gd.ImageB64 = convertImageToBase64(gd.Image)

		games[gd.ID] = &gd
	}

	return games
}

func convertImageToBase64(imagePath string) *string {
	if imagePath == "" {
		return nil
	}

	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		log.Printf("Error reading image file %s: %v", imagePath, err)

		return nil
	}
	base64Image := base64.StdEncoding.EncodeToString(imageData)

	return &base64Image
}
