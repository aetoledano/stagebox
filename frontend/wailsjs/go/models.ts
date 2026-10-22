export namespace audio {
	
	export class Device {
	    id: string;
	    name: string;
	    current: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Device(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.current = source["current"];
	    }
	}
	export class Status {
	    available: boolean;
	    deviceID: string;
	    deviceName: string;
	    volume: number;
	    muted: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.deviceID = source["deviceID"];
	        this.deviceName = source["deviceName"];
	        this.volume = source["volume"];
	        this.muted = source["muted"];
	    }
	}
	export class Settings {
	    devices: Device[];
	    status: Status;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.devices = this.convertValues(source["devices"], Device);
	        this.status = this.convertValues(source["status"], Status);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace bluetooth {
	
	export class PairedDevice {
	    id: string;
	    name: string;
	    address: string;
	    connected: boolean;
	    transport: string;
	
	    static createFrom(source: any = {}) {
	        return new PairedDevice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.address = source["address"];
	        this.connected = source["connected"];
	        this.transport = source["transport"];
	    }
	}
	export class PairedDeviceList {
	    devices: PairedDevice[];
	    complete: boolean;
	    warning: string;
	
	    static createFrom(source: any = {}) {
	        return new PairedDeviceList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.devices = this.convertValues(source["devices"], PairedDevice);
	        this.complete = source["complete"];
	        this.warning = source["warning"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace controller {
	
	export class ControllerInfo {
	    id: string;
	    name: string;
	    connection: string;
	    mode: string;
	    known: boolean;
	    connected: boolean;
	    active: boolean;
	    nativeXInput: boolean;
	    customMapping: boolean;
	    inputSource: number;
	
	    static createFrom(source: any = {}) {
	        return new ControllerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.connection = source["connection"];
	        this.mode = source["mode"];
	        this.known = source["known"];
	        this.connected = source["connected"];
	        this.active = source["active"];
	        this.nativeXInput = source["nativeXInput"];
	        this.customMapping = source["customMapping"];
	        this.inputSource = source["inputSource"];
	    }
	}
	export class ControllerMappingState {
	    id: string;
	    step: number;
	    total: number;
	    control: string;
	    phase: string;
	    binding: string;
	    message: string;
	    canSkip: boolean;
	    bindings: Record<string, string>;
	    command: string;
	
	    static createFrom(source: any = {}) {
	        return new ControllerMappingState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.step = source["step"];
	        this.total = source["total"];
	        this.control = source["control"];
	        this.phase = source["phase"];
	        this.binding = source["binding"];
	        this.message = source["message"];
	        this.canSkip = source["canSkip"];
	        this.bindings = source["bindings"];
	        this.command = source["command"];
	    }
	}

}

export namespace main {
	
	export class GameOptions {
	    id: string;
	    name: string;
	    enableVirtualInput: boolean;
	    virtualInputAvailable: boolean;
	    virtualInputMessage: string;
	
	    static createFrom(source: any = {}) {
	        return new GameOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.enableVirtualInput = source["enableVirtualInput"];
	        this.virtualInputAvailable = source["virtualInputAvailable"];
	        this.virtualInputMessage = source["virtualInputMessage"];
	    }
	}

}

export namespace wifi {
	
	export class Network {
	    ssid: string;
	    authentication: string;
	    encryption: string;
	    signal: number;
	    secure: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Network(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ssid = source["ssid"];
	        this.authentication = source["authentication"];
	        this.encryption = source["encryption"];
	        this.signal = source["signal"];
	        this.secure = source["secure"];
	    }
	}

}

