package entities

// VideoMetadata represents immutable information about a scraped video.
type VideoMetadata struct {
	id                string
	title             string
	encryptedEmbedURL string
	cleanSourceURL    string
}

// NewVideoMetadata creates a new immutable VideoMetadata instance.
func NewVideoMetadata(id, title, encryptedEmbedURL, cleanSourceURL string) VideoMetadata {
	return VideoMetadata{
		id:                id,
		title:             title,
		encryptedEmbedURL: encryptedEmbedURL,
		cleanSourceURL:    cleanSourceURL,
	}
}

// ID returns the video identifier.
func (v VideoMetadata) ID() string {
	return v.id
}

// Title returns the title of the video.
func (v VideoMetadata) Title() string {
	return v.title
}

// EncryptedEmbedURL returns the AES encrypted URL.
func (v VideoMetadata) EncryptedEmbedURL() string {
	return v.encryptedEmbedURL
}

// CleanSourceURL returns the decrypted, clean root server URL.
func (v VideoMetadata) CleanSourceURL() string {
	return v.cleanSourceURL
}
