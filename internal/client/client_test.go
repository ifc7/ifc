package client

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"testing"
)

func TestMain(m *testing.M) {
	// run from project root
	_, filename, _, _ := runtime.Caller(0)
	dir := path.Join(path.Dir(filename), "../..")
	err := os.Chdir(dir)
	if err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestIfc7ApiClient(t *testing.T) {
	t.Skip("skipping test because it requires valid credentials")
	client, err := NewAPIClient(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.ListInterfacesWithResponse(t.Context())
	if err != nil {
		t.Fatal(fmt.Errorf("failed to list interfaces: %w", err))
	}
	if response.StatusCode() != 200 {
		t.Fatal("expected status code 200")
	}
	fmt.Println(response.JSON200)
}
