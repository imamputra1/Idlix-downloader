package entities

type VideoMetadata struct {
	id                string
	title             string
	encryptedEmbedURL string
	decryptionKey     string
	cleanSourceURL    string
}

func NewVideoMetadata(id, title, encryptedEmbedURL, decryptionKey, cleanSourceURL string) VideoMetadata {
	return VideoMetadata{
		id:                id,
		title:             title,
		encryptedEmbedURL: encryptedEmbedURL,
		decryptionKey:     decryptionKey,
		cleanSourceURL:    cleanSourceURL,
	}
}

func (v VideoMetadata) ID() string {
	return v.id
}

func (v VideoMetadata) Title() string {
	return v.title
}

func (v VideoMetadata) EncryptedEmbedURL() string {
	return v.encryptedEmbedURL
}

func (v VideoMetadata) DecryptionKey() string {
	return v.decryptionKey
}

func (v VideoMetadata) CleanSourceURL() string {
	return v.cleanSourceURL
}
