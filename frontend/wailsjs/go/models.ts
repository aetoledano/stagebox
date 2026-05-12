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

