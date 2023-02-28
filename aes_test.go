package main

import "testing"

func TestEncryptAndDecrypt(t *testing.T) {

	{
		key, err := randomkey()
		if err != nil {
			t.Fatal(err)
		}

		encrypted, err := encrypt("test", key)
		if err != nil {
			t.Fatal(err)
		}

		decrypted, err := decrypt(encrypted, key)
		if err != nil {
			t.Fatal(err)
		}

		if decrypted != "test" {
			t.Error("decrypted should be 'test'")
		}

	}

	{
		key, err := randomkey()
		if err != nil {
			t.Fatal(err)
		}

		encrypted, err := encrypt("", key)
		if err != nil {
			t.Fatal(err)
		}

		if encrypted != "" {
			t.Error("encrypted should be empty")
		}

		decrypted, err := decrypt("", key)
		if err != nil {
			t.Fatal(err)
		}

		if decrypted != "" {
			t.Error("decrypted should be empty")
		}
	}
}
