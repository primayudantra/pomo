export namespace main {
	
	export class DaemonStatus {
	    running: boolean;
	    pid: number;
	
	    static createFrom(source: any = {}) {
	        return new DaemonStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.pid = source["pid"];
	    }
	}
	export class SessionRow {
	    task: string;
	    minutes: number;
	    status: string;
	    startedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task = source["task"];
	        this.minutes = source["minutes"];
	        this.status = source["status"];
	        this.startedAt = source["startedAt"];
	    }
	}
	export class State {
	    phase: string;
	    task: string;
	    remaining: number;
	    duration: number;
	    sessionId: number;
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.phase = source["phase"];
	        this.task = source["task"];
	        this.remaining = source["remaining"];
	        this.duration = source["duration"];
	        this.sessionId = source["sessionId"];
	    }
	}
	export class TodayView {
	    date: string;
	    focusMinutes: number;
	    count: number;
	    sessions: SessionRow[];
	
	    static createFrom(source: any = {}) {
	        return new TodayView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.focusMinutes = source["focusMinutes"];
	        this.count = source["count"];
	        this.sessions = this.convertValues(source["sessions"], SessionRow);
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

