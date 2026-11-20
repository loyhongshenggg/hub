export interface AppGeneratorSchema {
  id: string;
  name?: string;
  description?: string;
  template?: 'go-react' | 'image';
  image?: string;
  port?: number;
  healthPath?: string;
  skipInstall?: boolean;
}
