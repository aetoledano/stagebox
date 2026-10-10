package games

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestSteamUpdateRequestDoesNotLaunchGame(t *testing.T) {
	if args := steamGameUpdateArgs(42); !slices.Equal(args, []string{"-silent", "steam://install/42"}) {
		t.Fatalf("update request must not queue a game launch: %v", args)
	}
}

func TestLeaveSteamUpdateKeepsManifestAndCanReturnAgain(t *testing.T) {
	root := t.TempDir()
	manifest := steamManifestFixture(t, root, 42, 6, "Game", true)
	game, err := steamGameFromManifest(manifest, filepath.Join(root, "Steam.exe"))
	if err != nil || game == nil {
		t.Fatalf("create installed card: %v", err)
	}
	// Existing cards can return to their in-progress update; fresh discovery
	// and the actual launch path must still reject an active update.
	steamManifestFixture(t, root, 42, 1030, "Game", true)
	if found, err := steamGameFromManifest(manifest, game.Executable); err != nil || found != nil {
		t.Fatalf("discovery admitted an active update: game=%v err=%v", found, err)
	}
	if err := validateSteamGame(game); err == nil {
		t.Fatal("launch validation accepted an unfinished update")
	}
	prepared, err := PrepareSteamGame(game)
	if err != nil || !prepared.SteamNeedsUpdate {
		t.Fatalf("existing card could not wait for its update: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := WaitForSteamUpdate(ctx, prepared, func(status SteamLaunchStatus) {
			if !status.CanLeaveUpdate || !status.NeedsUpdate {
				t.Fatalf("update wait did not allow returning: %+v", status)
			}
			cancel()
		})
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("leaving update did not cancel only the wait: %v", err)
		}
		state, err := readSteamGameState(prepared)
		if err != nil || state.flags != 1030 {
			t.Fatalf("leaving modified Steam update state: %+v err=%v", state, err)
		}
	}
}

func TestSteamUpdateCompletionCanStillBeAbandoned(t *testing.T) {
	root := t.TempDir()
	manifest := steamManifestFixture(t, root, 42, 4, "Game", true)
	game, err := steamGameFromManifest(manifest, filepath.Join(root, "Steam.exe"))
	if err != nil || game == nil {
		t.Fatalf("create installed card: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = waitForSteamUpdate(ctx, game, steamGameState{flags: 6}, func(status SteamLaunchStatus) {
		if status.NeedsUpdate || !status.CanLeaveUpdate {
			t.Fatalf("completion stability wait lost the Back action: %+v", status)
		}
		cancel()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("completion beat an accepted cancellation: %v", err)
	}
}

func TestSteamUpdateCompletionReturnsToLaunchPreflight(t *testing.T) {
	root := t.TempDir()
	manifest := steamManifestFixture(t, root, 42, 4, "Game", true)
	game, err := steamGameFromManifest(manifest, filepath.Join(root, "Steam.exe"))
	if err != nil || game == nil {
		t.Fatalf("create installed card: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := waitForSteamUpdate(ctx, game, steamGameState{flags: 6}, nil); err != nil {
		t.Fatalf("completed installed update did not hand off: %v", err)
	}
}
