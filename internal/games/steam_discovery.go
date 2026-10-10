package games

import (
	"fmt"
	lgx "launcher/internal/logx"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

func FindSteamExecutable() string {
	candidates := []string{os.Getenv("STEAM_PATH"), os.Getenv("STEAM_DIR")}
	candidates = append(candidates, steamRegistryPaths()...)
	for _, variable := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
		if path := os.Getenv(variable); path != "" {
			candidates = append(candidates, filepath.Join(path, "Steam"))
		}
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		candidate = filepath.Clean(os.ExpandEnv(candidate))
		if !strings.EqualFold(filepath.Base(candidate), "steam.exe") {
			candidate = filepath.Join(candidate, "Steam.exe")
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
				candidate = resolved
			}
			if absolute, err := filepath.Abs(candidate); err == nil {
				return absolute
			}
		}
	}
	return ""
}

func discoverSteamGames(steamExe string, onError func(string, error)) []*GameData {
	return discoverSteamLibrary(steamExe, onError).games
}

type steamLibrarySnapshot struct {
	games    []*GameData
	present  map[uint32]bool
	complete bool
}

func discoverSteamLibrary(steamExe string, onError func(string, error)) steamLibrarySnapshot {
	snapshot := steamLibrarySnapshot{present: map[uint32]bool{}}
	if steamExe == "" {
		return snapshot
	}
	snapshot.complete = true
	report := func(path string, err error) {
		snapshot.complete = false
		lgx.Warnf("Steam library load failed path=%q err=%v", path, err)
		if onError != nil {
			onError(path, err)
		}
	}
	root := filepath.Dir(steamExe)
	libraries := []string{root}
	seenLibraries := map[string]bool{strings.ToLower(filepath.Clean(root)): true}
	for _, path := range []string{filepath.Join(root, "steamapps", "libraryfolders.vdf"), filepath.Join(root, "config", "libraryfolders.vdf")} {
		vdf, err := readSteamVDF(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			report(path, err)
			continue
		}
		folders := vdf.objects["libraryfolders"]
		if folders == nil {
			report(path, fmt.Errorf("missing libraryfolders object"))
			continue
		}
		for _, library := range steamLibraryPaths(folders) {
			key := strings.ToLower(filepath.Clean(library))
			if !seenLibraries[key] {
				libraries = append(libraries, library)
				seenLibraries[key] = true
			}
		}
	}
	result := []*GameData{}
	seenApps := map[uint32]bool{}
	for _, library := range libraries {
		appsDir := filepath.Join(library, "steamapps")
		entries, err := os.ReadDir(appsDir)
		if err != nil {
			report(appsDir, err)
			continue
		}
		for _, entry := range entries {
			name := strings.ToLower(entry.Name())
			if entry.IsDir() || !strings.HasPrefix(name, "appmanifest_") || !strings.HasSuffix(name, ".acf") {
				continue
			}
			path := filepath.Join(appsDir, entry.Name())
			id, parseErr := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(name, "appmanifest_"), ".acf"), 10, 32)
			if parseErr == nil && id != 0 {
				// Keep settings for manifests temporarily excluded by updates.
				snapshot.present[uint32(id)] = true
			}
			game, err := steamGameFromManifest(path, steamExe)
			if err != nil {
				report(path, err)
				continue
			}
			if game == nil || seenApps[game.SteamAppID] {
				continue
			}
			seenApps[game.SteamAppID] = true
			result = append(result, game)
		}
	}
	metadata := map[uint32]steamAppInfo{}
	var err error
	if len(result) > 0 {
		metadata, err = readSteamAppInfo(filepath.Join(root, "appcache", "appinfo.vdf"), seenApps)
	}
	if err != nil && !os.IsNotExist(err) {
		lgx.Warnf("Steam app metadata unavailable: %v; using installed manifests", err)
	}
	filtered := result[:0]
	for _, game := range result {
		info := metadata[game.SteamAppID]
		if info.appType != "" && info.appType != "game" && info.appType != "demo" && info.appType != "mod" {
			continue
		}
		if info.appType != "" {
			game.Controls = steamControlMode(info.controllerSupport)
		}
		filtered = append(filtered, game)
	}
	result = filtered
	lgx.Infof("Steam discovery libraries=%d installedGames=%d", len(libraries), len(result))
	snapshot.games = result
	return snapshot
}

func steamLibraryPaths(folders *steamVDF) []string {
	paths := []string{}
	for key, object := range folders.objects {
		if _, err := strconv.ParseUint(key, 10, 32); err == nil && filepath.IsAbs(object.values["path"]) {
			paths = append(paths, object.values["path"])
		}
	}
	// Older Steam clients store numeric keys with a path string directly.
	for key, path := range folders.values {
		if _, err := strconv.ParseUint(key, 10, 32); err == nil && filepath.IsAbs(path) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

func steamGameFromManifest(path, steamExe string) (*GameData, error) {
	return steamGameFromManifestForUpdate(path, steamExe, false)
}

func steamGameFromManifestForUpdate(path, steamExe string, allowUpdating bool) (*GameData, error) {
	vdf, err := readSteamVDF(path)
	if err != nil {
		return nil, err
	}
	state := vdf.objects["appstate"]
	if state == nil {
		return nil, fmt.Errorf("missing Steam AppState")
	}
	id, err := strconv.ParseUint(state.values["appid"], 10, 32)
	if err != nil || id == 0 {
		return nil, fmt.Errorf("invalid Steam app ID")
	}
	expected := fmt.Sprintf("appmanifest_%d.acf", id)
	if !strings.EqualFold(filepath.Base(path), expected) {
		return nil, fmt.Errorf("Steam app ID does not match manifest filename")
	}
	flags, err := strconv.ParseUint(state.values["stateflags"], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid Steam state flags")
	}
	// FullyInstalled is required; uninstalled, missing/corrupt files and active
	// downloads/uninstalls are excluded. UpdateRequired alone stays playable.
	incomplete := uint64(1 | 8 | 16 | 32 | 128 | 256 | 512 | 1024 | 2048 | 262144 | 524288 | 1048576 | 2097152 | 4194304)
	if allowUpdating {
		// Only an existing card may wait for an installed game's update.
		// Discovery still excludes partial installs and active updates.
		incomplete &^= steamUpdateActivityFlags | 512
	}
	if flags&4 == 0 || flags&incomplete != 0 {
		return nil, nil
	}
	name := strings.TrimSpace(state.values["name"])
	if name == "" || id == 228980 || strings.HasPrefix(strings.ToLower(name), "steam linux runtime") || strings.HasPrefix(strings.ToLower(name), "proton ") {
		return nil, nil
	}
	installDir := state.values["installdir"]
	if installDir == "" || installDir == "." || installDir == ".." || strings.ContainsAny(installDir, "/\\:") {
		return nil, fmt.Errorf("invalid Steam install directory")
	}
	installPath := filepath.Join(filepath.Dir(path), "common", installDir)
	entries, err := os.ReadDir(installPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	if resolved, err := filepath.EvalSymlinks(installPath); err == nil {
		installPath = resolved
	}
	appID := uint32(id)
	return &GameData{
		ID:         uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("launcher/steam/%d", appID))),
		SteamAppID: appID, SteamNeedsUpdate: flags&2 != 0, steamManifest: path, Name: name,
		Executable: steamExe, StartsOn: installPath, Args: steamGameLaunchArgs(appID),
		AdvancedTracking: true, TrackMode: TrackModeSteamGame, ShowCursor: true,
		// Steam manifests do not contain controller support metadata.
		Controls: -1,
	}, nil
}

func validateSteamGame(game *GameData) error {
	return validateSteamGameForUpdate(game, false)
}

func validateSteamGameForUpdate(game *GameData, allowUpdating bool) error {
	current, err := steamGameFromManifestForUpdate(game.steamManifest, game.Executable, allowUpdating)
	if err != nil {
		return err
	}
	if current == nil || current.SteamAppID != game.SteamAppID || !strings.EqualFold(current.StartsOn, game.StartsOn) {
		if state, stateErr := readSteamGameState(game); stateErr == nil && state.flags&(steamUpdateActivityFlags|512) != 0 {
			return fmt.Errorf("Steam is already updating this game. Open Steam to finish or resume the update, then try again")
		}
		return fmt.Errorf("Steam game is no longer fully installed; reload the library")
	}
	return nil
}
