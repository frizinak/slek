package slk

import "fmt"

func (s *Slk) join(e Entity) error {
	if e.Type() != TypeChannel {
		return fmt.Errorf("Can not join a %s", e.Type())
	}

	_, err := s.c.JoinChannel(e.Name())
	return err
}

func (s *Slk) leave(e Entity) error {
	if e.Type() != TypeChannel {
		return fmt.Errorf("Can not leave a %s", e.Type())
	}

	if e.(*channel).isChannel {
		_, err := s.c.LeaveChannel(e.ID())
		return err
	}

	return s.c.LeaveGroup(e.ID())
}

func (s *Slk) invite(chn, user Entity) error {
	if chn.Type() != TypeChannel {
		return fmt.Errorf("Can not invite someone to a %s", chn.Type())
	}

	if user.Type() != TypeUser {
		return fmt.Errorf("Can not invite a %s", user.Type())
	}

	if chn.(*channel).isChannel {
		_, err := s.c.InviteUserToChannel(chn.ID(), user.ID())
		return err
	}

	_, already, err := s.c.InviteUserToGroup(chn.ID(), user.ID())
	if err == nil && already {
		err = fmt.Errorf(
			"User %s is already in %s",
			user.Name(),
			chn.Name(),
		)
	}

	return err
}
