package slk

import "fmt"

func (s *Slk) join(e Entity) error {
	if e.GetType() != TypeChannel {
		return fmt.Errorf("Can not join a %s", e.GetType())
	}

	_, err := s.c.JoinChannel(e.GetName())
	return err
}

func (s *Slk) leave(e Entity) error {
	if e.GetType() != TypeChannel {
		return fmt.Errorf("Can not leave a %s", e.GetType())
	}

	_, err := s.c.LeaveChannel(e.GetID())
	return err
}

func (s *Slk) invite(chn, user Entity) error {
	if chn.GetType() != TypeChannel {
		return fmt.Errorf("Can not invite someone to a %s", chn.GetType())
	}

	if user.GetType() != TypeUser {
		return fmt.Errorf("Can not invite a %s", user.GetType())
	}

	if chn.(*channel).isChannel {
		_, err := s.c.InviteUserToChannel(chn.GetID(), user.GetID())
		return err
	}

	_, already, err := s.c.InviteUserToGroup(chn.GetID(), user.GetID())
	if err == nil && already {
		err = fmt.Errorf(
			"User %s is already in %s",
			user.GetName(),
			chn.GetName(),
		)
	}

	return err
}
