// Legacy response shapes only. There is no cloud runtime product entrypoint.
export interface CloudRuntimeNode {
  id: string;
  owner_id: string;
  instance_id: string;
  region: string;
  instance_type: string;
  image_id: string;
  subnet_id: string;
  name: string;
  status: string;
  tags: Record<string, string>;
  metadata: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface ListCloudRuntimeNodesParams {
  limit?: number;
  offset?: number;
}

export interface CreateCloudRuntimeNodeRequest {
  instance_type: string;
  name?: string;
  region?: string;
  image_id?: string;
  subnet_id?: string;
  key_name?: string;
  iam_instance_profile?: string;
  disk_size_gb?: number;
  tags?: Record<string, string>;
}
