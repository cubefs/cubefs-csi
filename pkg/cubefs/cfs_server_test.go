package cubefs

import "testing"

func TestNormalizeReplicaNum(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "empty defaults to 3", want: "3"},
		{name: "one", in: "1", want: "1"},
		{name: "two", in: "2", want: "2"},
		{name: "three", in: "3", want: "3"},
		{name: "zero", in: "0", wantErr: true},
		{name: "four", in: "4", wantErr: true},
		{name: "word", in: "three", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			param := map[string]string{}
			if tc.in != "" {
				param[KReplicaNum] = tc.in
			}
			err := normalizeReplicaNum(param)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if param[KReplicaNum] != tc.want {
				t.Fatalf("replicaNum = %q, want %q", param[KReplicaNum], tc.want)
			}
		})
	}
}

func TestNewCfsServerReplicaNum(t *testing.T) {
	cs, err := newCfsServer("pvc-a", map[string]string{
		KMasterAddr: "10.0.0.1:17010",
		KReplicaNum: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cs.clientConf[KReplicaNum] != "1" {
		t.Fatalf("replicaNum = %q", cs.clientConf[KReplicaNum])
	}

	if _, err = newCfsServer("pvc-b", map[string]string{
		KMasterAddr: "10.0.0.1:17010",
		KReplicaNum: "5",
	}); err == nil {
		t.Fatal("expected invalid replicaNum to fail")
	}
}
