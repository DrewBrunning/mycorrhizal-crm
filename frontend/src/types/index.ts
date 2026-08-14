export interface User {
  id: number;
  email: string;
  username: string;
  language: string;
  is_admin: boolean;
  created_at: string;
  updated_at: string;
  // Present only on the /users/me response (CurrentUserResponse), not in admin lists.
  // null means the user has never configured it; apply DEFAULT_ENABLED_CONTACT_FIELDS.
  enabled_contact_fields?: string[] | null;
  // Present only on the /users/me response. VCardUID of the caller's "Me"
  // contact (T90); null/absent means none is set.
  self_contact_vcard_uid?: string | null;
}

export interface UsersListResponse {
  users: User[];
  total: number;
  page: number;
  limit: number;
  total_pages: number;
}

export interface UserUpdateInput {
  username?: string;
  email?: string;
  password?: string;
  is_admin?: boolean;
}

export interface UserCreateInput {
  username: string;
  email: string;
  password: string;
  is_admin?: boolean;
}
