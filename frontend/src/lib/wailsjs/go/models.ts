export namespace config {
	
	export class Config {
	    baseUrl: string;
	    apiKey: string;
	    chatModel: string;
	    titleModel: string;
	    systemPrompt: string;
	
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
	    }
	}

}

export namespace db {
	
	export class Message {
	    id: string;
	    threadId: string;
	    role: string;
	    content: string;
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
	        this.createdAt = source["createdAt"];
	    }
	}
	export class Thread {
	    id: string;
	    title: string;
	    systemPrompt: string;
	    temperature: number;
	    topP: number;
	    maxTokens: number;
	    frequencyPenalty: number;
	    createdAt: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Thread(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.systemPrompt = source["systemPrompt"];
	        this.temperature = source["temperature"];
	        this.topP = source["topP"];
	        this.maxTokens = source["maxTokens"];
	        this.frequencyPenalty = source["frequencyPenalty"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}

}

