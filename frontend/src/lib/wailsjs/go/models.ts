export namespace config {
	
	export class Config {
	    baseUrl: string;
	    apiKey: string;
	    chatModel: string;
	    titleModel: string;
	    systemPrompt: string;
	    contextWindow: number;
	    temperature?: number;
	    topP?: number;
	    maxTokens?: number;
	    frequencyPenalty?: number;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.baseUrl = source["baseUrl"];
	        this.apiKey = source["apiKey"];
	        this.chatModel = source["chatModel"];
	        this.titleModel = source["titleModel"];
	        this.systemPrompt = source["systemPrompt"];
	        this.contextWindow = source["contextWindow"];
	        this.temperature = source["temperature"];
	        this.topP = source["topP"];
	        this.maxTokens = source["maxTokens"];
	        this.frequencyPenalty = source["frequencyPenalty"];
	    }
	}

}

export namespace db {
	
	export class Attachment {
	    id: string;
	    messageId: string;
	    name: string;
	    mimeType: string;
	    size: number;
	    dataUrl?: string;
	    createdAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Attachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.messageId = source["messageId"];
	        this.name = source["name"];
	        this.mimeType = source["mimeType"];
	        this.size = source["size"];
	        this.dataUrl = source["dataUrl"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class AttachmentInput {
	    name: string;
	    mimeType: string;
	    data: string;
	
	    static createFrom(source: any = {}) {
	        return new AttachmentInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.mimeType = source["mimeType"];
	        this.data = source["data"];
	    }
	}
	export class Message {
	    id: string;
	    threadId: string;
	    role: string;
	    content: string;
	    reasoning: string;
	    promptTokens: number;
	    completionTokens: number;
	    reasoningTokens: number;
	    attachments?: Attachment[];
	    createdAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.threadId = source["threadId"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.reasoning = source["reasoning"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.reasoningTokens = source["reasoningTokens"];
	        this.attachments = this.convertValues(source["attachments"], Attachment);
	        this.createdAt = source["createdAt"];
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
	export class SearchResult {
	    threadId: string;
	    title: string;
	    snippet: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.threadId = source["threadId"];
	        this.title = source["title"];
	        this.snippet = source["snippet"];
	    }
	}
	export class Thread {
	    id: string;
	    title: string;
	    model: string;
	    systemPrompt: string;
	    temperature?: number;
	    topP?: number;
	    maxTokens?: number;
	    frequencyPenalty?: number;
	    reasoningEffort: string;
	    contextWindow?: number;
	    createdAt: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Thread(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.model = source["model"];
	        this.systemPrompt = source["systemPrompt"];
	        this.temperature = source["temperature"];
	        this.topP = source["topP"];
	        this.maxTokens = source["maxTokens"];
	        this.frequencyPenalty = source["frequencyPenalty"];
	        this.reasoningEffort = source["reasoningEffort"];
	        this.contextWindow = source["contextWindow"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class ThreadSettings {
	    systemPrompt: string;
	    temperature?: number;
	    topP?: number;
	    maxTokens?: number;
	    frequencyPenalty?: number;
	    reasoningEffort: string;
	    contextWindow?: number;
	
	    static createFrom(source: any = {}) {
	        return new ThreadSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.systemPrompt = source["systemPrompt"];
	        this.temperature = source["temperature"];
	        this.topP = source["topP"];
	        this.maxTokens = source["maxTokens"];
	        this.frequencyPenalty = source["frequencyPenalty"];
	        this.reasoningEffort = source["reasoningEffort"];
	        this.contextWindow = source["contextWindow"];
	    }
	}

}

export namespace main {
	
	export class AboutInfo {
	    title: string;
	    version: string;
	    description: string;
	    copyright: string;
	    icon: string;
	
	    static createFrom(source: any = {}) {
	        return new AboutInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.version = source["version"];
	        this.description = source["description"];
	        this.copyright = source["copyright"];
	        this.icon = source["icon"];
	    }
	}

}

