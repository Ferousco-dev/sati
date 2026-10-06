package config

import "testing"

func TestValidate(t *testing.T) {
	t.Run("requires database url", func(t *testing.T) {
		if err := (Config{AppEnv: "development"}).validate(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("development needs only database url", func(t *testing.T) {
		if err := (Config{AppEnv: "development", DatabaseURL: "postgres://x"}).validate(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("production refuses missing secrets", func(t *testing.T) {
		if err := (Config{AppEnv: "production", DatabaseURL: "postgres://x"}).validate(); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestParsePrefixes(t *testing.T) {
	got, err := parsePrefixes("127.0.0.1, 10.0.0.0/8")
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := parsePrefixes("not-an-ip"); err == nil {
		t.Fatal("expected error")
	}
}
