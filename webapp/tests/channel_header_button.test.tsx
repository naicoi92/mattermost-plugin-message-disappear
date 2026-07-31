// Component test for the channel-header TTL dropdown button (MPMD-32).
// The component receives the channel as a prop (passed by registerChannelHeaderIcon's
// Pluggable); the TTL hook and the client are mocked, so we assert pure behaviour.
import {fireEvent, render, screen, within} from '@testing-library/react';

jest.mock('client', () => ({
    setTTL: jest.fn(),
    clearTTL: jest.fn(),
    getTTL: jest.fn(),
}));

let mockTtlValue: {duration: number; set_by: string; set_at: number} | null = null;
jest.mock('hooks/use_channel_ttl', () => ({
    useChannelTTL: () => mockTtlValue,
}));

import {setTTL, clearTTL} from 'client';
import ChannelHeaderButton from 'components/channel_header_button';

const props = {channel: {id: 'ch-current'}};
const toggle = () => screen.getByRole('button', {name: /disappearing/i});

beforeEach(() => {
    (setTTL as jest.Mock).mockClear();
    (clearTTL as jest.Mock).mockClear();
    mockTtlValue = null;
});

it('shows "Off" when no TTL is set, and the duration when one is', () => {
    const {rerender} = render(<ChannelHeaderButton {...props}/>);
    expect(toggle()).toHaveTextContent('Off');

    mockTtlValue = {duration: 86400, set_by: 'u', set_at: 1};
    rerender(<ChannelHeaderButton {...props}/>);
    expect(toggle()).toHaveTextContent('1d');
});

it('opens the preset dropdown on click (Off + presets, no Custom)', () => {
    render(<ChannelHeaderButton {...props}/>);
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    fireEvent.click(toggle());

    const menu = screen.getByRole('menu');
    expect(within(menu).getByRole('menuitem', {name: /off/i})).toBeInTheDocument();
    expect(within(menu).getByRole('menuitem', {name: '1d'})).toBeInTheDocument();
    expect(within(menu).getByRole('menuitem', {name: '1w'})).toBeInTheDocument();
    // No Custom entry — quick presets only.
    expect(screen.queryByRole('menuitem', {name: /custom/i})).not.toBeInTheDocument();
});

it('selecting a preset sets the channel TTL with the right duration and closes', () => {
    render(<ChannelHeaderButton {...props}/>);
    fireEvent.click(toggle());
    fireEvent.click(screen.getByRole('menuitem', {name: '1d'}));
    expect(setTTL).toHaveBeenCalledWith('ch-current', 86400);
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
});

it('selecting Off clears the TTL', () => {
    render(<ChannelHeaderButton {...props}/>);
    fireEvent.click(toggle());
    fireEvent.click(screen.getByRole('menuitem', {name: /^off$/i}));
    expect(clearTTL).toHaveBeenCalledWith('ch-current');
});

it('closes on Escape and on outside click', () => {
    render(<ChannelHeaderButton {...props}/>);
    fireEvent.click(toggle());
    expect(screen.getByRole('menu')).toBeInTheDocument();

    fireEvent.keyDown(document, {key: 'Escape'});
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();

    fireEvent.click(toggle());
    expect(screen.getByRole('menu')).toBeInTheDocument();
    fireEvent.mouseDown(document.body);
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
});
