package gym

import "testing"

func TestCountResult(t *testing.T) {
	count, err := countResult("testdata/centos7-primary.sqlite", "")
	if err != nil {
		t.Fatal(err)
	}
	if count != 8652 {
		t.Errorf("count should be %d, but is %d", 8652, count)
	}
	count, err = countResult("testdata/centos7-primary.sqlite", "zsh")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("count should be %d, but is %d", 2, count)
	}
}

func TestTotalBytes(t *testing.T) {
	total, err := totalBytes("testdata/centos7-primary.sqlite", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 24408969744 {
		t.Errorf("count should be %d, but is %d", 24408969744, total)
	}
	total, err = totalBytes("testdata/centos7-primary.sqlite", "zsh")
	if err != nil {
		t.Fatal(err)
	}
	if total != 9172632 {
		t.Errorf("count should be %d, but is %d", 9172632, total)
	}
}

func TestEllipsis(t *testing.T) {
	max := 6
	okString := "123456"
	okSmallString := "123"
	nokString := "1234567"

	if ellipsis(okString, max) != okString {
		t.Errorf("expected value %s, got %s", okString, ellipsis(okString, max))
	}

	if ellipsis(okSmallString, max) != "123..." {
		t.Errorf("expected value '%s', got %s", "123...", ellipsis(okSmallString, max))
	}

	if ellipsis(nokString, max) != "123456" {
		t.Errorf("expected value %s, got %s", "123456", ellipsis(nokString, max))
	}

	max = 0
	if ellipsis(nokString, max) != "" {
		t.Errorf("expected value %s, got %s", "", ellipsis(nokString, max))
	}
}
