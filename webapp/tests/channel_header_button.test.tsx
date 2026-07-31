// Component test for the channel-header TTL dropdown button (MPMD-32).
// The component is fully isolated: react-redux, the TTL hook and the client are
// mocked, so we assert pure render + interaction behaviour.
import {fireEvent, render, screen, within} from '@testing-library/react';

jest.mock('react-redux', () => ({
    useSelector: (selector: (s: unknown) => unknown) => selector({
        entities: {
            channels: {currentChannelId: 'ch-current'},
            posts: {posts: {}},
        },
    }),
    useDispatch: () => mockDispatch,
}));

jest.mock('client', () => ({
    setTTL: jest.fn(),
    clearTTL: jest.fn(),
    getTTL: jest.fn(),
}));

let mockTtlValue: {duration: number; set_by: string; set_at: number} | null = null;
jest.mock('hooks/use_channel_ttl', () => ({
    useChannelTTL: () => mockTtlValue,
}));

// Imported after jest.mock so they resolve to the mocked fns.
import {setTTL, clearTTL} from 'client';
import {openModal} from 'reducer';
import ChannelHeaderButton from 'components/channel_header_button';

const mockDispatch = jest.fn();

const toggle = () => screen.getByRole('button', {name: /disappearing/i});

beforeEach(() => {
    (setTTL as jest.Mock).mockClear();
    (clearTTL as jest.Mock).mockClear();
    mockDispatch.mockClear();
    mockTtlValue = null;
});

it('shows "Off" label when no TTL is set, and the duration when one is', () => {
    const {rerender} = render(<ChannelHeaderButton/>);
    expect(toggle()).toHaveTextContent('Off');

    mockTtlValue = {duration: 86400, set_by: 'u', set_at: 1};
    rerender(<ChannelHeaderButton/>);
    expect(toggle()).toHaveTextContent('1d');
});

it('opens the preset dropdown on click and lists Off + presets + Custom', () => {
    render(<ChannelHeaderButton/>);
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    fireEvent.click(toggle());
    const menu = screen.getByRole('menu');
    expect(menu).toBeInTheDocument();
    expect(within(menu).getByRole('button', {name: /off/i})).toBeInTheDocument();
    expect(within(menu).getByRole('button', {name: '1d'})).toBeInTheDocument();
    expect(within(menu).getByRole('button', {name: '1w'})).toBeInTheDocument();
    expect(within(menu).getByRole('button', {name: /custom/i})).toBeInTheDocument();
});

it('selecting a preset sets the channel TTL with the right duration and closes', async () => {
    render(<ChannelHeaderButton/>);
    fireEvent.click(toggle());
    fireEvent.click(screen.getByRole('button', {name: '1d'}));
    expect(setTTL).toHaveBeenCalledWith('ch-current', 86400);
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
});

it('selecting Off clears the TTL', () => {
    render(<ChannelHeaderButton/>);
    fireEvent.click(toggle());
    fireEvent.click(screen.getByRole('button', {name: /^off$/i}));
    expect(clearTTL).toHaveBeenCalledWith('ch-current');
});

it('selecting Custom opens the duration modal', () => {
    render(<ChannelHeaderButton/>);
    fireEvent.click(toggle());
    fireEvent.click(screen.getByRole('button', {name: /custom/i}));
    expect(mockDispatch).toHaveBeenCalledWith(openModal('ch-current'));
});

it('closes on Escape and on outside click', () => {
    render(<ChannelHeaderButton/>);
    fireEvent.click(toggle());
    expect(screen.getByRole('menu')).toBeInTheDocument();

    fireEvent.keyDown(document, {key: 'Escape'});
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();

    fireEvent.click(toggle());
    expect(screen.getByRole('menu')).toBeInTheDocument();
    fireEvent.mouseDown(document.body);
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
});
