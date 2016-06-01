package crypto

type ChallengeProvider interface {
	DeriveKey(string, []byte) []byte
	RandomSalt() []byte
	Challenge(message []byte, key []byte) []byte
}
