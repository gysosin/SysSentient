import React from 'react';
import { Link } from 'react-router-dom';
import { KeyRound, LogOut, UserRound } from 'lucide-react';

import { AuthUser } from '../services/api';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from './ui/dropdown-menu';

export const UserMenu: React.FC<{ user: AuthUser; onSignOut: () => void }> = ({
  user,
  onSignOut,
}) => (
  <DropdownMenu>
    <DropdownMenuTrigger asChild>
      <Button
        variant="ghost"
        size="sm"
        aria-label={`Account menu for ${user.email}`}
        className="gap-2"
      >
        <UserRound className="size-4" aria-hidden="true" />
        <span className="hidden max-w-[14rem] truncate sm:inline">{user.email}</span>
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end">
      <DropdownMenuLabel className="flex items-center justify-between gap-3">
        <span className="truncate">{user.email}</span>
        <Badge variant="outline" className="px-1.5 py-0 text-2xs uppercase">
          {user.role}
        </Badge>
      </DropdownMenuLabel>
      <DropdownMenuSeparator />
      <DropdownMenuItem asChild>
        <Link to="/settings#account">
          <KeyRound className="size-4" aria-hidden="true" />
          Change password
        </Link>
      </DropdownMenuItem>
      <DropdownMenuItem onSelect={onSignOut}>
        <LogOut className="size-4" aria-hidden="true" />
        Sign out
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
);
