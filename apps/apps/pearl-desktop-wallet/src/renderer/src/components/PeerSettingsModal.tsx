import { useState, useEffect } from 'react';
import { X, RotateCcw, Save } from 'lucide-react';
import { Button } from '@modelos/ui/components/button';

interface PeerSettingsModalProps {
    isOpen: boolean;
    onClose: () => void;
}

export function PeerSettingsModal({ isOpen, onClose }: PeerSettingsModalProps) {
    const [peerAddress, setPeerAddress] = useState('');
    const [peerPort, setPeerPort] = useState('');
    const [defaultAddress, setDefaultAddress] = useState('');
    const [defaultPort, setDefaultPort] = useState('');
    const [isCustom, setIsCustom] = useState(false);
    const [isSaving, setIsSaving] = useState(false);
    const [network, setNetwork] = useState('');

    useEffect(() => {
        if (isOpen) {
            loadPeerSettings();
        }
    }, [isOpen]);

    const loadPeerSettings = async () => {
        try {
            const settings = await window.appBridge.manager.getPeerSettings();
            setPeerAddress(settings.currentAddress);
            setPeerPort(String(settings.currentPort));
            setDefaultAddress(settings.defaultAddress);
            setDefaultPort(String(settings.defaultPort));
            setIsCustom(settings.isCustom);
            setNetwork(settings.network);
        } catch (error) {
            console.error('Failed to load peer settings:', error);
        }
    };

    const handleSave = async () => {
        setIsSaving(true);
        try {
            const port = parseInt(peerPort, 10);
            if (isNaN(port) || port < 1 || port > 65535) {
                alert('Invalid port number');
                return;
            }

            await window.appBridge.manager.setCustomPeerAddress(peerAddress, port);
            alert('Peer settings saved! Please restart the wallet for changes to take effect.');
            onClose();
        } catch (error) {
            console.error('Failed to save peer settings:', error);
            alert('Failed to save settings');
        } finally {
            setIsSaving(false);
        }
    };

    const handleReset = async () => {
        setIsSaving(true);
        try {
            await window.appBridge.manager.resetPeerToDefault();
            await loadPeerSettings();
            alert('Peer settings reset to default! Please restart the wallet for changes to take effect.');
        } catch (error) {
            console.error('Failed to reset peer settings:', error);
            alert('Failed to reset settings');
        } finally {
            setIsSaving(false);
        }
    };

    if (!isOpen) return null;

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
            <div className="w-full max-w-md rounded-lg border border-gray-300 bg-white p-6 shadow-xl">
                {/* Header */}
                <div className="mb-4 flex items-center justify-between">
                    <div className="flex items-center gap-2">
                        <h2 className="text-xl font-bold text-gray-900">Peer Settings</h2>
                        {network && (
                            <span className={`rounded-full px-2 py-0.5 text-xs font-semibold capitalize ${network === 'mainnet' ? 'bg-green-100 text-green-700' : 'bg-amber-100 text-amber-700'}`}>
                                {network}
                            </span>
                        )}
                    </div>
                    <button
                        onClick={onClose}
                        className="rounded-full p-1 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700"
                    >
                        <X className="h-5 w-5" />
                    </button>
                </div>

                {/* Form */}
                <div className="space-y-4">
                    <div>
                        <label className="mb-1 block text-sm font-medium text-gray-700">Peer Address</label>
                        <input
                            type="text"
                            value={peerAddress}
                            onChange={(e) => setPeerAddress(e.target.value)}
                            placeholder="127.0.0.1 or IP address"
                            className="w-full rounded-lg border border-gray-300 px-3 py-2 text-gray-900 focus:border-green-500 focus:outline-none focus:ring-2 focus:ring-green-500/20"
                        />
                    </div>

                    <div>
                        <label className="mb-1 block text-sm font-medium text-gray-700">Peer Port</label>
                        <input
                            type="number"
                            value={peerPort}
                            onChange={(e) => setPeerPort(e.target.value)}
                            placeholder="8333"
                            className="w-full rounded-lg border border-gray-300 px-3 py-2 text-gray-900 focus:border-green-500 focus:outline-none focus:ring-2 focus:ring-green-500/20"
                        />
                    </div>

                    {isCustom && (
                        <div className="rounded-lg bg-blue-50 p-3">
                            <p className="text-sm text-blue-700">
                                <strong>Default:</strong> {defaultAddress}:{defaultPort}
                            </p>
                        </div>
                    )}

                    {/* Actions */}
                    <div className="flex gap-3">
                        <Button
                            onClick={handleReset}
                            variant="outline"
                            disabled={isSaving}
                            className="flex-1"
                        >
                            <RotateCcw className="mr-2 h-4 w-4" />
                            Reset to Default
                        </Button>

                        <Button
                            onClick={handleSave}
                            disabled={isSaving || !peerAddress.trim() || !peerPort.trim()}
                            className="flex-1 bg-green-600 hover:bg-green-700"
                        >
                            <Save className="mr-2 h-4 w-4" />
                            {isSaving ? 'Saving...' : 'Save'}
                        </Button>
                    </div>

                    <p className="text-xs text-gray-500">
                        Note: You'll need to restart the wallet after changing peer settings.
                    </p>
                </div>
            </div>
        </div>
    );
}

