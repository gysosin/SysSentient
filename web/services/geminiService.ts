import { GoogleGenAI, Type } from '@google/genai';
import { SystemMetrics, Process, LogEntry, AIAnalysisResult } from '../types';
import { scrubObject } from './piiScrubber';
import { GEMINI_MODEL } from '../constants';

const getAiClient = () => {
    // In a real app, strict error handling for missing key is needed.
    // For this demo, we assume the environment is set up correctly.
    const apiKey = process.env.API_KEY || '';
    return new GoogleGenAI({ apiKey });
};

export const analyzeSystemHealth = async (
  metrics: SystemMetrics,
  processes: Process[],
  logs: LogEntry[]
): Promise<AIAnalysisResult> => {
  
  if (!process.env.API_KEY) {
      console.warn("No API KEY found");
      return {
          status: 'Warning',
          summary: "API Key Missing",
          detailedAnalysis: "Please configure process.env.API_KEY to enable AI features.",
          recommendedActions: []
      };
  }

  const ai = getAiClient();
  
  // Prepare Data Payload
  const payload = {
    metrics,
    top_processes: processes.slice(0, 5), // Only send top 5 to save tokens
    recent_logs: logs
  };

  // PII Scrubbing
  const safePayload = scrubObject(payload);

  const prompt = `
    You are SysSentient, an advanced AI Linux System Administrator.
    Analyze the following system snapshot (JSON) and provide a diagnosis.
    
    Snapshot:
    ${JSON.stringify(safePayload, null, 2)}
    
    Your goal is to explain WHY the system is behaving this way in plain English.
    Identify any critical issues (High CPU, OOM kills, Disk Thrashing).
    Suggest concrete actions (commands) to fix them.
    
    Respond in JSON format conforming to this schema:
    {
      "status": "Healthy" | "Warning" | "Critical",
      "summary": "Short 1 sentence summary",
      "detailedAnalysis": "Detailed explanation (max 3 sentences)",
      "recommendedActions": [
        {
          "id": "unique_id",
          "command": "shell command to run",
          "description": "what this does",
          "isSafe": boolean
        }
      ]
    }
  `;

  try {
    const response = await ai.models.generateContent({
      model: GEMINI_MODEL,
      contents: prompt,
      config: {
        responseMimeType: 'application/json',
        responseSchema: {
          type: Type.OBJECT,
          properties: {
            status: { type: Type.STRING, enum: ['Healthy', 'Warning', 'Critical'] },
            summary: { type: Type.STRING },
            detailedAnalysis: { type: Type.STRING },
            recommendedActions: {
              type: Type.ARRAY,
              items: {
                type: Type.OBJECT,
                properties: {
                  id: { type: Type.STRING },
                  command: { type: Type.STRING },
                  description: { type: Type.STRING },
                  isSafe: { type: Type.BOOLEAN }
                },
                required: ['id', 'command', 'description', 'isSafe']
              }
            }
          },
          required: ['status', 'summary', 'detailedAnalysis', 'recommendedActions']
        }
      }
    });

    const text = response.text || "{}";
    const result = JSON.parse(text) as AIAnalysisResult;
    return result;

  } catch (error) {
    console.error("Gemini Analysis Failed:", error);
    return {
      status: 'Warning',
      summary: "AI Analysis Failed",
      detailedAnalysis: "Could not connect to Gemini API. Please check your network or API key.",
      recommendedActions: []
    };
  }
};
