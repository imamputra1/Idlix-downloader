package entities_test

import (
	"testing"

	"github.com/imamputra1/idlix-downloader/internal/core/entities"
)

func TestVideoMetadata_CreationAccuracy(t *testing.T) {
	video := entities.NewVideoMetadata("123", "Title", "embed_url", "dummy-key", "clean_url")
	if video.ID() != "1" || video.Title() != "Title" || video.EncryptedEmbedURL() != "enc" || video.CleanSourceURL() != "clean" {
		t.Fatal("VideoMetadata immutability or constructor failed")
	}
}

func TestHLSPlaylist_MutationResistance(t *testing.T) {
	orig := []entities.MediaSegment{entities.NewMediaSegment("a.ts", 0, 1.0)}
	playlist := entities.NewHLSPlaylist("720p", 1.0, orig)

	orig[0] = entities.NewMediaSegment("hacked.ts", 99, 99.0)
	if playlist.Segments()[0].URL() == "hacked.ts" {
		t.Fatal("Breach: Mutated via original slice")
	}

	ext := playlist.Segments()
	ext[0] = entities.NewMediaSegment("hacked2.ts", 99, 99.0)
	if playlist.Segments()[0].URL() == "hacked2.ts" {
		t.Fatal("Breach: Mutated via getter slice")
	}
}

func TestEntities_ZeroValues(t *testing.T) {
	t.Run("VideoMetadata Zero Values", func(t *testing.T) {
		video := entities.NewVideoMetadata("", "", "", "", "")

		if video.ID() != "" || video.Title() != "" {
			t.Errorf("Expected empty strings for zero-value VideoMetadata initialization")
		}
	})

	t.Run("HLSPlaylist Zero Values", func(t *testing.T) {
		playlistNil := entities.NewHLSPlaylist("", 0.0, nil)
		if len(playlistNil.Segments()) != 0 {
			t.Errorf("Expected empty segments slice when initialized with nil")
		}

		playlistEmpty := entities.NewHLSPlaylist("", 0.0, []entities.MediaSegment{})
		if len(playlistEmpty.Segments()) != 0 {
			t.Errorf("Expected empty segments slice when initialized with empty slice")
		}
	})
}
