package identity

import (
	"strings"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestCredentials(t *testing.T) {
	t.Parallel()
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Local Credential Contract")
}

var _ = ginkgo.Describe("Local credentials", func() {
	ginkgo.It("uses one Unicode key without changing display spelling", func() {
		for _, pair := range [][2]string{
			{"Anna", "anna"}, {"Straße", "STRASSE"}, {"équipe", "e\u0301quipe"},
		} {
			left, err := UsernameKey(pair[0])
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			right, err := UsernameKey(pair[1])
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(left).To(gomega.Equal(right))
		}
		accented, err := UsernameKey("équipe")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		plain, err := UsernameKey("equipe")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(accented).NotTo(gomega.Equal(plain))
	})

	ginkgo.DescribeTable("rejects rather than trimming or truncating usernames", func(value string) {
		_, err := UsernameKey(value)
		gomega.Expect(err).To(gomega.MatchError(ErrInvalidInput))
	},
		ginkgo.Entry("empty", ""),
		ginkgo.Entry("leading whitespace", " Anna"),
		ginkgo.Entry("trailing Unicode whitespace", "Anna\u00a0"),
		ginkgo.Entry("control character", "An\nna"),
		ginkgo.Entry("invalid UTF-8", string([]byte{0xff})),
		ginkgo.Entry("too long", strings.Repeat("a", 129)),
	)

	ginkgo.It("salts hashes and preserves exact password bytes", func() {
		password := "é-strong-password"
		first, err := HashPassword(password)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		second, err := HashPassword(password)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(first).NotTo(gomega.Equal(second))
		valid, err := VerifyPassword(first, password)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(valid).To(gomega.BeTrue())
		valid, err = VerifyPassword(first, "e\u0301-strong-password")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(valid).To(gomega.BeFalse())
		valid, err = VerifyPassword(first, "wrong-password")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(valid).To(gomega.BeFalse())
	})

	ginkgo.It("encodes the production cost parameters", func() {
		gomega.Expect(ProductionPasswordParams).To(gomega.Equal(PasswordParams{MemoryKiB: 19 * 1024, Iterations: 2, Parallelism: 1}))
		encoded, err := HashPassword("correct-password")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(encoded).To(gomega.HavePrefix("$argon2id$v=19$m=19456,t=2,p=1$"))
		zero, err := PasswordHasher{}.Hash("correct-password")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(zero).To(gomega.HavePrefix("$argon2id$v=19$m=19456,t=2,p=1$"))
	})

	ginkgo.It("lets a reduced-cost hasher verify its own and production hashes only", func() {
		cheap, err := NewPasswordHasher(PasswordParams{MemoryKiB: 8, Iterations: 1, Parallelism: 1})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		own, err := cheap.Hash("correct-password")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(own).To(gomega.HavePrefix("$argon2id$v=19$m=8,t=1,p=1$"))
		valid, err := cheap.Verify(own, "correct-password")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(valid).To(gomega.BeTrue())
		production, err := HashPassword("correct-password")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		valid, err = cheap.Verify(production, "correct-password")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(valid).To(gomega.BeTrue())
		// The production verifier must not accept reduced-cost hashes.
		_, err = VerifyPassword(own, "correct-password")
		gomega.Expect(err).To(gomega.HaveOccurred())
		_, err = NewPasswordHasher(PasswordParams{})
		gomega.Expect(err).To(gomega.MatchError(ErrInvalidInput))
	})

	ginkgo.It("rejects corrupt or unbounded hash parameters explicitly", func() {
		for _, stored := range []string{"", "$argon2id$v=19$m=999999999,t=2,p=1$AA$AA"} {
			_, err := VerifyPassword(stored, "correct-password")
			gomega.Expect(err).To(gomega.HaveOccurred())
		}
		gomega.Expect(ValidatePassword("short")).To(gomega.MatchError(ErrInvalidInput))
		gomega.Expect(ValidatePassword(strings.Repeat("x", 1025))).To(gomega.MatchError(ErrInvalidInput))
		gomega.Expect(ValidatePassword(strings.Repeat("x", 1024))).To(gomega.Succeed())
	})

	ginkgo.It("generates canonical secrets with independently verifiable hashes", func() {
		first, hash, err := NewSecret()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(first).To(gomega.HaveLen(43))
		actual, err := SecretHash(first)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(actual).To(gomega.Equal(hash))
		second, _, err := NewSecret()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(second).NotTo(gomega.Equal(first))
		_, err = SecretHash(first + "=")
		gomega.Expect(err).To(gomega.MatchError(ErrLoginRequired))
	})
})
