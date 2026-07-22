package sf

import "fmt"

// UserIdentity is the cached "who am I" pair for the current org's
// CLI session. Id is the User.Id (005…); Name is the display name
// (FirstName + LastName, server-rendered).
//
// Built so chip predicates that need to reference "the current user"
// can do so without a baked-in literal — the user id is what
// records-shaped server-side chips want; the display name is what
// client-side row matchers (e.g. /flows filtering on
// LastModifiedBy / CreatedBy strings) need.
type UserIdentity struct {
	ID   string
	Name string
}

// CurrentUserIdentity resolves both the Id and Name for the current
// CLI session's username in one SOQL. Cached on HomeData by the
// caller so we only fire it once per home fetch per org.
func CurrentUserIdentity(target, username string) (UserIdentity, error) {
	if username == "" {
		return UserIdentity{}, fmt.Errorf("CurrentUserIdentity: username is empty")
	}
	soql := fmt.Sprintf(
		"SELECT Id, Name FROM User WHERE Username = '%s' LIMIT 1",
		sqlEscape(username))
	q, err := Query(target, soql, false)
	if err != nil {
		return UserIdentity{}, err
	}
	if len(q.Records) == 0 {
		return UserIdentity{}, fmt.Errorf("CurrentUserIdentity: no User row matched %s", username)
	}
	id := asString(q.Records[0]["Id"])
	if id == "" {
		return UserIdentity{}, fmt.Errorf("CurrentUserIdentity: empty Id in User row")
	}
	return UserIdentity{
		ID:   id,
		Name: asString(q.Records[0]["Name"]),
	}, nil
}
