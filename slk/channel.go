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

// Mark the last read message in an IM, channel or group.
func (s *Slk) mark(e Entity) error {
	var err error

	latest := e.latest()
	switch e.Type() {
	case TypeChannel:
		if e.(*channel).isChannel {
			err = s.c.SetChannelReadMark(e.ID(), latest)
			latest = e.latest()
			break
		}

		err = s.c.SetGroupReadMark(e.ID(), latest)
	case TypeUser:
		err = s.c.MarkIMChannel(s.imByUser(e.ID()).ID, latest)
	default:
		err = fmt.Errorf("Can't mark a %s", e.Type())
	}

	if err != nil {
		s.out.Warn(err.Error())
		return err
	}

	e.setLastRead(latest)

	return nil
}
