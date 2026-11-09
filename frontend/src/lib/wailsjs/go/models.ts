export namespace config {
	
	export class Provider {
	    id: string;
	    name: string;
	    baseUrl: string;
	    headers?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new Provider(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.baseUrl = source["baseUrl"];
	        this.headers = source["headers"];
	    }
	}
	export class Config {
	    theme: string;
	    activeProvider: string;
	    activeModel: string;
	    systemPrompt: string;
	    providers: Provider[];
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	        this.activeProvider = source["activeProvider"];
	        this.activeModel = source["activeModel"];
	        this.systemPrompt = source["systemPrompt"];
	        this.providers = this.convertValues(source["providers"], Provider);
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

