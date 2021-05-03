package wire

import "io"

// fakeMessage implements the Message interface and is used to force encode errors in messages.
type fakeMessage struct {
	command        string
	payload        []byte
	forceEncodeErr bool
	forceLenErr    bool
}

// BtcDecode doesn't do anything.  It just satisfies the wire.Message interface.
func (msg *fakeMessage) BtcDecode(r io.Reader, pver uint32, enc MessageEncoding) (e error) {
	return nil
}

// BtcEncode writes the payload field of the fake message or forces an error if the forceEncodeErr flag of the fake
// message is set. It also satisfies the wire.Message interface.
func (msg *fakeMessage) BtcEncode(w io.Writer, pver uint32, enc MessageEncoding) (e error) {
	if msg.forceEncodeErr {
		e := &MessageError{
			Func:        "fakeMessage.BtcEncode",
			Description: "intentional error",
		}
		return e
	}
	_, e := w.Write(msg.payload)
	return e
}

// Command returns the command field of the fake message and satisfies the Message interface.
func (msg *fakeMessage) Command() string {
	return msg.command
}

// MaxPayloadLength returns the length of the payload field of fake message or a smaller value if the forceLenErr flag
// of the fake message is set. It satisfies the Message interface.
func (msg *fakeMessage) MaxPayloadLength(pver uint32) uint32 {
	lenp := uint32(len(msg.payload))
	if msg.forceLenErr {
		return lenp - 1
	}
	return lenp
}