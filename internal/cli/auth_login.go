// Copyright 2026 Ayoub and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"database/sql"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"

	"meetup-pp-cli/internal/config"
)

const meetupCookieName = "__meetup_auth_access_token"

func newAuthLoginCmd(flags *rootFlags) *cobra.Command {
	var flagBrowser string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Extract your Meetup session token from your browser automatically.",
		Long: `Reads the ` + meetupCookieName + ` cookie from your
Chrome, Brave, or Firefox browser. You must already be logged into meetup.com.

The session token expires after ~1 hour. Run 'auth login' again to refresh it.`,
		Example: `  meetup-pp-cli auth login
  meetup-pp-cli auth login --browser firefox
  meetup-pp-cli auth login --browser brave`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if runtime.GOOS != "darwin" {
				return fmt.Errorf("auth login is only supported on macOS\nOn other platforms, use: meetup-pp-cli auth set-token <token>")
			}

			token, source, err := extractMeetupCookie(flagBrowser)
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "Could not read browser cookies automatically.")
				fmt.Fprintln(cmd.ErrOrStderr(), "")
				fmt.Fprintln(cmd.ErrOrStderr(), "Manual fallback:")
				fmt.Fprintln(cmd.ErrOrStderr(), "  1. Open meetup.com in Chrome and log in")
				fmt.Fprintln(cmd.ErrOrStderr(), "  2. DevTools → Network → filter 'gql' → click a request → Request Headers")
				fmt.Fprintln(cmd.ErrOrStderr(), "  3. Copy the cookie value for __meetup_auth_access_token")
				fmt.Fprintln(cmd.ErrOrStderr(), "  4. Run: meetup-pp-cli auth set-token <token>")
				return fmt.Errorf("browser cookie extraction failed: %w", err)
			}

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			cfg.AuthHeaderVal = ""
			if err := cfg.SaveTokens("", "", token, "", cfg.TokenExpiry); err != nil {
				return configErr(fmt.Errorf("saving token: %w", err))
			}

			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"logged_in": true,
					"browser":   source,
					"note":      "token expires in ~1 hour, re-run auth login to refresh",
				}, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Logged in via %s. Token saved to config.\n", source)
			fmt.Fprintln(cmd.OutOrStdout(), "Note: session token expires in ~1 hour — re-run 'auth login' to refresh.")
			return nil
		},
	}
	cmd.Flags().StringVar(&flagBrowser, "browser", "", "Browser to read from: chrome, brave, firefox (default: try all)")
	return cmd
}

// extractMeetupCookie tries each browser in order and returns the first token found.
func extractMeetupCookie(browser string) (token, source string, err error) {
	type candidate struct {
		name string
		fn   func() (string, error)
	}
	all := []candidate{
		{"Chrome", readChromeCookie("Google/Chrome", "Chrome Safe Storage")},
		{"Brave", readChromeCookie("BraveSoftware/Brave-Browser", "Brave Safe Storage")},
		{"Firefox", readFirefoxCookie},
	}

	if browser != "" {
		b := strings.ToLower(browser)
		for _, c := range all {
			if strings.ToLower(c.name) == b {
				t, err := c.fn()
				if err != nil {
					return "", "", err
				}
				return t, c.name, nil
			}
		}
		return "", "", fmt.Errorf("unknown browser %q — choose: chrome, brave, firefox", browser)
	}

	var errs []string
	for _, c := range all {
		t, e := c.fn()
		if e == nil && t != "" {
			return t, c.name, nil
		}
		if e != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", c.name, e))
		}
	}
	return "", "", fmt.Errorf("no Meetup session found (%s)", strings.Join(errs, "; "))
}

// readChromeCookie returns a function that reads the cookie from a Chromium-based browser.
func readChromeCookie(appPath, keychainService string) func() (string, error) {
	return func() (string, error) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		baseDir := filepath.Join(home, "Library", "Application Support", appPath)

		// Collect all profile cookie files (Default + Profile N).
		var dbPaths []string
		entries, _ := os.ReadDir(baseDir)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			n := e.Name()
			if n == "Default" || strings.HasPrefix(n, "Profile") {
				p := filepath.Join(baseDir, n, "Cookies")
				if _, err := os.Stat(p); err == nil {
					dbPaths = append(dbPaths, p)
				}
			}
		}
		if len(dbPaths) == 0 {
			return "", fmt.Errorf("%s not installed or no profiles found", keychainService)
		}

		for _, dbPath := range dbPaths {
			tmp, err := copyToTemp(dbPath, "meetup-chrome-*.sqlite")
			if err != nil {
				continue
			}
			db, err := sql.Open("sqlite", "file:"+tmp+"?mode=ro&immutable=1")
			if err != nil {
				os.Remove(tmp)
				continue
			}
			var enc []byte
			err = db.QueryRow(
				`SELECT encrypted_value FROM cookies
				 WHERE host_key LIKE '%.meetup.com' AND name = ? LIMIT 1`,
				meetupCookieName,
			).Scan(&enc)
			db.Close()
			os.Remove(tmp)
			if err != nil {
				continue
			}
			return decryptChromeCookie(enc, keychainService)
		}
		return "", fmt.Errorf("cookie not found in %s (are you logged into meetup.com?)", keychainService)
	}
}

// decryptChromeCookie decrypts a macOS Chrome v10-encrypted cookie value.
func decryptChromeCookie(enc []byte, keychainService string) (string, error) {
	if len(enc) < 3 || string(enc[:3]) != "v10" {
		// Unencrypted (rare but possible on older profiles)
		return string(enc), nil
	}

	// Get password from macOS Keychain
	out, err := exec.Command("security", "find-generic-password", "-w", "-s", keychainService).Output()
	if err != nil {
		return "", fmt.Errorf("keychain access denied for %q — grant Terminal Full Disk Access in System Settings → Privacy & Security", keychainService)
	}
	password := strings.TrimSpace(string(out))

	// Derive 16-byte AES key via PBKDF2-HMAC-SHA1
	key := pbkdf2Key([]byte(password), []byte("saltysalt"), 1003, 16)

	ciphertext := enc[3:]
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	iv := []byte("                ") // 16 spaces
	if len(ciphertext)%aes.BlockSize != 0 {
		return "", fmt.Errorf("ciphertext is not a multiple of AES block size")
	}
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(ciphertext, ciphertext)

	// Remove PKCS7 padding
	if len(ciphertext) == 0 {
		return "", fmt.Errorf("decrypted value is empty")
	}
	pad := int(ciphertext[len(ciphertext)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(ciphertext) {
		return "", fmt.Errorf("invalid padding")
	}
	plaintext := ciphertext[:len(ciphertext)-pad]

	// Chrome prepends 32 bytes of internal metadata before the actual cookie
	// value. Find the real content by looking for the first printable ASCII
	// run, or for 'eyJ' which is the base64url prefix of every JWT.
	if idx := bytes.Index(plaintext, []byte("eyJ")); idx > 0 {
		plaintext = plaintext[idx:]
	} else {
		// Generic fallback: skip leading non-printable bytes.
		for len(plaintext) > 0 && (plaintext[0] < 0x20 || plaintext[0] > 0x7e) {
			plaintext = plaintext[1:]
		}
	}
	return string(plaintext), nil
}

// readFirefoxCookie reads the plaintext cookie from Firefox's SQLite store.
func readFirefoxCookie() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	profiles, err := filepath.Glob(
		filepath.Join(home, "Library", "Application Support", "Firefox", "Profiles", "*", "cookies.sqlite"),
	)
	if err != nil || len(profiles) == 0 {
		return "", fmt.Errorf("Firefox profile not found")
	}

	tmp, err := copyToTemp(profiles[0], "meetup-ff-*.sqlite")
	if err != nil {
		return "", fmt.Errorf("cannot read Firefox cookies: %w", err)
	}
	defer os.Remove(tmp)

	db, err := sql.Open("sqlite", "file:"+tmp+"?mode=ro&immutable=1")
	if err != nil {
		return "", err
	}
	defer db.Close()

	var value string
	err = db.QueryRow(
		`SELECT value FROM moz_cookies
		 WHERE host LIKE '%.meetup.com' AND name = ? LIMIT 1`,
		meetupCookieName,
	).Scan(&value)
	if err != nil {
		return "", fmt.Errorf("cookie not found in Firefox (are you logged into meetup.com?): %w", err)
	}
	return value, nil
}

// copyToTemp copies a file to a temp path and returns the temp path.
func copyToTemp(src, pattern string) (string, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	tmp.Close()
	return tmp.Name(), nil
}

// pbkdf2Key derives a key using PBKDF2-HMAC-SHA1 (no external dependency).
func pbkdf2Key(password, salt []byte, iter, keyLen int) []byte {
	h := hmac.New(sha1.New, password)
	dk := make([]byte, 0, keyLen)
	for block := 1; len(dk) < keyLen; block++ {
		h.Reset()
		h.Write(salt)
		var buf [4]byte
		binary.BigEndian.PutUint32(buf[:], uint32(block))
		h.Write(buf[:])
		T := h.Sum(nil)
		U := append([]byte(nil), T...)
		for n := 1; n < iter; n++ {
			h.Reset()
			h.Write(U)
			U = h.Sum(U[:0])
			for i := range T {
				T[i] ^= U[i]
			}
		}
		dk = append(dk, T...)
	}
	return dk[:keyLen]
}
