package hey

import "github.com/basecamp/hey-sdk/go/pkg/generated"

// DraftSentAs answers the contact a saved draft sends as, the way HEY reckons it
// (Entry#sent_as): its sender, else its creator. HEY names a sender only when it
// differs from the creator, so a draft sent from the person's own address carries a
// creator and no sender, and that creator need not be the account's default sender.
// Its Id is the ActingSenderID a revision of the draft carries; a zero Contact means
// HEY named neither.
func DraftSentAs(edit *generated.MessageEditState) generated.Contact {
	if edit == nil {
		return generated.Contact{}
	}
	return sentAs(edit.Sender, edit.Creator)
}

// ReplySentAs answers the contact a reply goes out as, from NewReply's prefill: the
// sender HEY resolved from the thread when it differs from the acting user, else the
// creator, the acting user's own contact. Its Id is the actingSenderID CreateReply and
// CreateReplyDraft take. Passing the prefill's Sender.Id alone sends zero for a
// reply from the person's own address, and zero falls back to the account's default
// sender, which need not be that address.
func ReplySentAs(prefill *generated.MessageDraft) generated.Contact {
	if prefill == nil {
		return generated.Contact{}
	}
	return sentAs(prefill.Sender, prefill.Creator)
}

func sentAs(sender, creator generated.Contact) generated.Contact {
	if sender.Id != 0 {
		return sender
	}
	return creator
}

// DraftContentFromEdit is a draft's edit state as the whole DraftContent a revision
// sends back: its subject, body, recipients, the identity it is sent as
// (DraftSentAs) and its scheduled delivery. HEY revises a draft from the whole of what
// is sent, so an edit starts here and changes only what it means to: a hand-built
// DraftContent left with a zero ActingSenderID moves the draft to the account's
// default sender, and one with a nil Schedule unschedules it.
//
// The schedule is said back as the served moment's own UTC date and hour, which is how
// HEY's API reads a schedule's date and hour. HEY schedules to the hour, so a moment
// between whole hours (set from a HEY app on a fractional-offset clock) comes back
// truncated to its hour; a caller that must not move it checks the minutes first.
func DraftContentFromEdit(edit *generated.MessageEditState) DraftContent {
	if edit == nil {
		return DraftContent{}
	}
	content := DraftContent{
		Subject:        edit.Subject,
		Content:        edit.Content,
		To:             contactAddresses(edit.Addressed.Directly),
		CC:             contactAddresses(edit.Addressed.Copied),
		BCC:            contactAddresses(edit.Addressed.Blindcopied),
		ActingSenderID: DraftSentAs(edit).Id,
	}
	if !edit.ScheduledDeliveryAt.IsZero() {
		at := edit.ScheduledDeliveryAt.UTC()
		content.Schedule = &DraftSchedule{Date: at.Format("2006-01-02"), Hour: at.Hour()}
	}
	return content
}

// contactAddresses is the email addresses of contacts, skipping any HEY served
// without one.
func contactAddresses(contacts []generated.Contact) []string {
	var addresses []string
	for _, contact := range contacts {
		if contact.EmailAddress != "" {
			addresses = append(addresses, contact.EmailAddress)
		}
	}
	return addresses
}
