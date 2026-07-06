export namespace main {
	
	export class JobView {
	    uuid: string;
	    name: string;
	    schedule: string;
	    paused: boolean;
	    lastRunStatus: string;
	    lastRunAt: string;
	    running: boolean;
	
	    static createFrom(source: any = {}) {
	        return new JobView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uuid = source["uuid"];
	        this.name = source["name"];
	        this.schedule = source["schedule"];
	        this.paused = source["paused"];
	        this.lastRunStatus = source["lastRunStatus"];
	        this.lastRunAt = source["lastRunAt"];
	        this.running = source["running"];
	    }
	}
	export class RegisterResult {
	    agentUuid: string;
	    agentApiKey: string;
	    bucket: string;
	    accessKey: string;
	    secretKey: string;
	    syncEngine: string;
	
	    static createFrom(source: any = {}) {
	        return new RegisterResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agentUuid = source["agentUuid"];
	        this.agentApiKey = source["agentApiKey"];
	        this.bucket = source["bucket"];
	        this.accessKey = source["accessKey"];
	        this.secretKey = source["secretKey"];
	        this.syncEngine = source["syncEngine"];
	    }
	}
	export class StatusView {
	    registered: boolean;
	    agentUuid: string;
	    bucket: string;
	    syncEngine: string;
	    daemonUp: boolean;
	    daemonErr?: string;
	    jobs: JobView[];
	
	    static createFrom(source: any = {}) {
	        return new StatusView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.registered = source["registered"];
	        this.agentUuid = source["agentUuid"];
	        this.bucket = source["bucket"];
	        this.syncEngine = source["syncEngine"];
	        this.daemonUp = source["daemonUp"];
	        this.daemonErr = source["daemonErr"];
	        this.jobs = this.convertValues(source["jobs"], JobView);
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

