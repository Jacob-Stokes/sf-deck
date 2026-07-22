package soqlfmt

import "testing"

func TestFormat(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "simple",
			in:   "select id, name from account limit 20",
			want: "SELECT id, name\nFROM account\nLIMIT 20",
		},
		{
			name: "where + order",
			in:   "select id from contact where account.industry='Technology' and createddate=last_n_days:30 order by lastmodifieddate desc limit 50",
			want: "SELECT id\nFROM contact\nWHERE account.industry = 'Technology'\n  AND createddate = last_n_days:30\nORDER BY lastmodifieddate DESC\nLIMIT 50",
		},
		{
			name: "preserves field-name case",
			in:   "SELECT Id, MyCustom__c, Account.Name FROM Account",
			want: "SELECT Id, MyCustom__c, Account.Name\nFROM Account",
		},
		{
			name: "strings stay intact",
			in:   "SELECT Id FROM Account WHERE Name = 'O''Brien'",
			want: "SELECT Id\nFROM Account\nWHERE Name = 'O''Brien'",
		},
		{
			name: "in clause",
			in:   "SELECT Id FROM Account WHERE Industry IN ('Tech','Health')",
			want: "SELECT Id\nFROM Account\nWHERE Industry IN ('Tech', 'Health')",
		},
		{
			name: "group by",
			in:   "select count(id), industry from account group by industry having count(id) > 10",
			want: "SELECT COUNT(id), industry\nFROM account\nGROUP BY industry\nHAVING COUNT(id) > 10",
		},
		{
			name: "idempotent",
			in:   "SELECT Id\nFROM Account\nLIMIT 20",
			want: "SELECT Id\nFROM Account\nLIMIT 20",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Format(tc.in)
			if got != tc.want {
				t.Errorf("Format mismatch\n--- got ---\n%s\n--- want ---\n%s", got, tc.want)
			}
		})
	}
}
